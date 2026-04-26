// Ring topology e2e test for CrossTalk SFU audio forwarding.
//
// Runs against a real ct-server in Docker. Creates 4 synthetic WebRTC
// clients, connects them in a ring (A→B→C→D→A), and validates that
// non-silence RTP packets flow all the way around.
//
// Tests multiple join orderings to catch timing bugs.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"nhooyr.io/websocket"
)

var serverURL string

func main() {
	serverURL = os.Getenv("CT_SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Printf("ring-test: server=%s", serverURL)

	waitForServer(30 * time.Second)

	token := getSeedToken()
	log.Printf("ring-test: got API token")

	passed := 0
	failed := 0

	tests := []struct {
		name      string
		joinOrder []int
		delays    []time.Duration
	}{
		{
			name:      "all-join-simultaneously",
			joinOrder: []int{0, 1, 2, 3},
			delays:    []time.Duration{0, 0, 0, 0},
		},
		{
			name:      "reverse-join-order",
			joinOrder: []int{3, 2, 1, 0},
			delays:    []time.Duration{0, 100 * time.Millisecond, 100 * time.Millisecond, 100 * time.Millisecond},
		},
		{
			name:      "client-2-joins-first-client-3-last",
			joinOrder: []int{2, 0, 1, 3},
			delays:    []time.Duration{0, 200 * time.Millisecond, 200 * time.Millisecond, 500 * time.Millisecond},
		},
		{
			name:      "interleaved-join",
			joinOrder: []int{0, 3, 1, 2},
			delays:    []time.Duration{0, 100 * time.Millisecond, 300 * time.Millisecond, 100 * time.Millisecond},
		},
	}

	for _, tt := range tests {
		log.Printf("\n=== TEST: %s ===", tt.name)
		err := runRingTest(token, tt.joinOrder, tt.delays)
		if err != nil {
			log.Printf("FAIL: %s: %v", tt.name, err)
			failed++
		} else {
			log.Printf("PASS: %s", tt.name)
			passed++
		}
	}

	log.Printf("\n=== RESULTS: %d passed, %d failed ===", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// ── Server API helpers ──

func waitForServer(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(serverURL + "/api/connections")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Fatal("server not ready")
}

func getSeedToken() string {
	body := `{"username":"admin","password":"Password!"}`
	resp, err := http.Post(serverURL+"/api/auth/login", "application/json", strings.NewReader(body))
	if err != nil {
		log.Fatalf("login failed: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		log.Fatalf("login failed: %d %s", resp.StatusCode, string(data))
	}
	var result struct {
		Token string `json:"token"`
	}
	json.Unmarshal(data, &result)
	return result.Token
}

func apiCall(method, path string, token string, body interface{}) (map[string]interface{}, error) {
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, serverURL+path, bodyReader)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API %s %s: %d %s", method, path, resp.StatusCode, string(data))
	}
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	return result, nil
}

func createAPIToken(sessionToken string) (string, error) {
	result, err := apiCall("POST", "/api/tokens", sessionToken, map[string]string{
		"name": fmt.Sprintf("ring-test-%d", time.Now().UnixMilli()),
	})
	if err != nil {
		return "", err
	}
	return result["token"].(string), nil
}

// ── WebRTC client ──

type ringClient struct {
	name      string
	role      string
	marker    byte
	pc        *webrtc.PeerConnection
	ws        *websocket.Conn
	sendTrack *webrtc.TrackLocalStaticRTP
	received  chan []byte
	controlCh chan []byte
	connected chan struct{}
}

func newRingClient(name, role string, marker byte, apiToken string) (*ringClient, error) {
	var se webrtc.SettingEngine
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	api := webrtc.NewAPI(webrtc.WithSettingEngine(se))

	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, fmt.Errorf("create pc: %w", err)
	}

	sendTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		fmt.Sprintf("%s-mic", role),
		fmt.Sprintf("%s-mic", role),
	)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("create track: %w", err)
	}
	if _, err := pc.AddTrack(sendTrack); err != nil {
		pc.Close()
		return nil, fmt.Errorf("add track: %w", err)
	}

	dc, err := pc.CreateDataChannel("control", nil)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("create dc: %w", err)
	}

	rc := &ringClient{
		name:      name,
		role:      role,
		marker:    marker,
		pc:        pc,
		sendTrack: sendTrack,
		received:  make(chan []byte, 1000),
		controlCh: make(chan []byte, 100),
		connected: make(chan struct{}),
	}

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		select {
		case rc.controlCh <- msg.Data:
		default:
		}
	})

	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Printf("%s: OnTrack fired: id=%s codec=%s", name, track.ID(), track.Codec().MimeType)
		for {
			pkt, _, err := track.ReadRTP()
			if err != nil {
				return
			}
			payload := make([]byte, len(pkt.Payload))
			copy(payload, pkt.Payload)
			select {
			case rc.received <- payload:
			default:
			}
		}
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("%s: ICE %s", name, state.String())
		if state == webrtc.ICEConnectionStateConnected {
			select {
			case <-rc.connected:
			default:
				close(rc.connected)
			}
		}
	})

	wsURL := strings.Replace(serverURL, "http://", "ws://", 1) + "/ws/signaling?token=" + apiToken
	ctx := context.Background()
	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("ws dial: %w", err)
	}
	rc.ws = ws

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		cJSON := c.ToJSON()
		cBytes, _ := json.Marshal(cJSON)
		msg, _ := json.Marshal(map[string]interface{}{
			"type":      "ice",
			"candidate": json.RawMessage(cBytes),
		})
		ws.Write(ctx, websocket.MessageText, msg)
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return nil, fmt.Errorf("create offer: %w", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return nil, fmt.Errorf("set local desc: %w", err)
	}

	offerMsg, _ := json.Marshal(map[string]string{
		"type": "offer",
		"sdp":  offer.SDP,
	})
	if err := ws.Write(ctx, websocket.MessageText, offerMsg); err != nil {
		return nil, fmt.Errorf("send offer: %w", err)
	}

	go rc.readSignalingLoop(ctx)

	select {
	case <-rc.connected:
	case <-time.After(15 * time.Second):
		return nil, fmt.Errorf("%s: ICE connect timeout", name)
	}

	return rc, nil
}

func (rc *ringClient) readSignalingLoop(ctx context.Context) {
	for {
		_, data, err := rc.ws.Read(ctx)
		if err != nil {
			return
		}
		var msg struct {
			Type      string          `json:"type"`
			SDP       string          `json:"sdp"`
			Candidate json.RawMessage `json:"candidate"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "answer":
			answer := webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: msg.SDP}
			if err := rc.pc.SetRemoteDescription(answer); err != nil {
				log.Printf("%s: set answer failed: %v", rc.name, err)
			}

		case "offer":
			offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: msg.SDP}
			if err := rc.pc.SetRemoteDescription(offer); err != nil {
				log.Printf("%s: set renegotiation offer failed: %v", rc.name, err)
				continue
			}
			ans, err := rc.pc.CreateAnswer(nil)
			if err != nil {
				log.Printf("%s: create answer failed: %v", rc.name, err)
				continue
			}
			if err := rc.pc.SetLocalDescription(ans); err != nil {
				log.Printf("%s: set local desc failed: %v", rc.name, err)
				continue
			}
			ansMsg, _ := json.Marshal(map[string]string{
				"type": "answer",
				"sdp":  ans.SDP,
			})
			rc.ws.Write(ctx, websocket.MessageText, ansMsg)
			log.Printf("%s: sent renegotiation answer", rc.name)

		case "ice":
			if len(msg.Candidate) == 0 || string(msg.Candidate) == "null" {
				continue
			}
			var init webrtc.ICECandidateInit
			if err := json.Unmarshal(msg.Candidate, &init); err != nil {
				continue
			}
			rc.pc.AddICECandidate(init)
		}
	}
}

func (rc *ringClient) sendHello() {
	hello := map[string]interface{}{
		"hello": map[string]interface{}{
			"sources": []map[string]string{{"name": rc.role + "-mic", "type": "audio"}},
			"sinks":   []map[string]string{{"name": rc.role + "-speaker", "type": "audio"}},
			"codecs":  []map[string]string{{"name": "opus/48000/2", "media_type": "audio"}},
		},
	}
	data, _ := json.Marshal(hello)

	dc := rc.findControlDC()
	if dc == nil {
		log.Printf("%s: no control DC for Hello", rc.name)
		return
	}
	dc.Send(data)
	log.Printf("%s: sent Hello", rc.name)
}

func (rc *ringClient) sendJoinSession(sessionID string) {
	join := map[string]interface{}{
		"joinSession": map[string]string{
			"session_id": sessionID,
			"role":       rc.role,
		},
	}
	data, _ := json.Marshal(join)

	dc := rc.findControlDC()
	if dc == nil {
		log.Printf("%s: no control DC for JoinSession", rc.name)
		return
	}
	dc.Send(data)
	log.Printf("%s: sent JoinSession role=%s", rc.name, rc.role)
}

func (rc *ringClient) findControlDC() *webrtc.DataChannel {
	// The server creates a "control" DC; we created one too.
	// Use the one we created since we have a direct reference.
	// Actually — we need to find the server's DC that was negotiated.
	// Let's use the OnDataChannel approach instead.
	return nil
}

func (rc *ringClient) startSending(ctx context.Context) {
	go func() {
		seq := uint16(1)
		ts := uint32(960)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			payload := bytes.Repeat([]byte{rc.marker}, 20)
			pkt := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    111,
					SequenceNumber: seq,
					Timestamp:      ts,
					SSRC:           uint32(rc.marker) * 1000,
				},
				Payload: payload,
			}
			rc.sendTrack.WriteRTP(pkt)
			seq++
			ts += 960
			time.Sleep(20 * time.Millisecond)
		}
	}()
}

func (rc *ringClient) waitForPayloads(marker byte, count int, timeout time.Duration) int {
	expected := bytes.Repeat([]byte{marker}, 20)
	got := 0
	deadline := time.After(timeout)
	for got < count {
		select {
		case p := <-rc.received:
			if bytes.Equal(p, expected) {
				got++
			}
		case <-deadline:
			return got
		}
	}
	return got
}

func (rc *ringClient) close() {
	if rc.ws != nil {
		rc.ws.Close(websocket.StatusNormalClosure, "done")
	}
	if rc.pc != nil {
		rc.pc.Close()
	}
}

// ── Test runner ──

func runRingTest(sessionToken string, joinOrder []int, delays []time.Duration) error {
	apiToken, err := createAPIToken(sessionToken)
	if err != nil {
		return fmt.Errorf("create API token: %w", err)
	}

	tmplResult, err := apiCall("POST", "/api/templates", sessionToken, map[string]interface{}{
		"name": fmt.Sprintf("ring-4-%d", time.Now().UnixNano()),
		"roles": []map[string]interface{}{
			{"name": "alpha", "multi_client": false},
			{"name": "beta", "multi_client": false},
			{"name": "gamma", "multi_client": false},
			{"name": "delta", "multi_client": false},
		},
		"mappings": []map[string]string{
			{"source": "alpha:mic", "sink": "beta:speaker"},
			{"source": "beta:mic", "sink": "gamma:speaker"},
			{"source": "gamma:mic", "sink": "delta:speaker"},
			{"source": "delta:mic", "sink": "alpha:speaker"},
		},
	})
	if err != nil {
		return fmt.Errorf("create template: %w", err)
	}
	tmplID := tmplResult["id"].(string)
	log.Printf("  template created: %s", tmplID)

	sessResult, err := apiCall("POST", "/api/sessions", sessionToken, map[string]string{
		"template_id": tmplID,
		"name":        "ring-test",
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	sessionID := sessResult["id"].(string)
	log.Printf("  session created: %s", sessionID)

	roles := []string{"alpha", "beta", "gamma", "delta"}
	markers := []byte{0xA1, 0xB2, 0xC3, 0xD4}

	clients := make([]*ringClient, 4)
	for i := 0; i < 4; i++ {
		c, err := newRingClient(roles[i], roles[i], markers[i], apiToken)
		if err != nil {
			return fmt.Errorf("create client %s: %w", roles[i], err)
		}
		clients[i] = c
	}
	defer func() {
		for _, c := range clients {
			if c != nil {
				c.close()
			}
		}
	}()

	log.Printf("  all 4 clients connected to server")

	// The clients need to send Hello + JoinSession via the control data channel.
	// Since the control DC is created by the server and delivered via OnDataChannel,
	// we need to wait for it. However, the test clients created their own "control"
	// DC in the offer. The server should recognize it.
	//
	// Actually — looking at the server code, the server creates its own "control"
	// DC and the client's DC arrives via OnDataChannel. The JSON control messages
	// need to go through the server's DC. Since we're sending via WS signaling
	// and the control channel, let me use the REST API to assign peers instead.

	// Wait for connections to stabilize
	time.Sleep(1 * time.Second)

	// Get peer IDs from server
	connsResult, err := apiCall("GET", "/api/connections", sessionToken, nil)
	if err != nil {
		log.Printf("  warning: could not list connections: %v", err)
	} else {
		log.Printf("  connections: %v", connsResult)
	}

	// Use the REST API to assign peers to the session
	// First, list connections to get peer IDs
	resp, err := http.NewRequest("GET", serverURL+"/api/connections", nil)
	if err == nil {
		resp.Header.Set("Authorization", "Bearer "+sessionToken)
		r, err := http.DefaultClient.Do(resp)
		if err == nil {
			data, _ := io.ReadAll(r.Body)
			r.Body.Close()
			log.Printf("  connections response: %s", string(data))

			var conns []map[string]interface{}
			json.Unmarshal(data, &conns)

			if len(conns) >= 4 {
				for i, joinIdx := range joinOrder {
					if i > 0 && delays[i] > 0 {
						time.Sleep(delays[i])
					}

					peerID := conns[joinIdx]["id"].(string)
					_, err := apiCall("POST", fmt.Sprintf("/api/sessions/%s/assign", sessionID), sessionToken, map[string]string{
						"peer_id": peerID,
						"role":    roles[joinIdx],
					})
					if err != nil {
						return fmt.Errorf("assign %s: %w", roles[joinIdx], err)
					}
					log.Printf("  assigned %s → %s (peer %s)", roles[joinIdx], sessionID, peerID)
				}
			} else {
				return fmt.Errorf("expected 4 connections, got %d", len(conns))
			}
		}
	}

	// Wait for bindings to activate and tracks to propagate
	time.Sleep(2 * time.Second)

	// Start sending from all clients
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, c := range clients {
		c.startSending(ctx)
	}

	// Wait for forwarding to start
	time.Sleep(3 * time.Second)

	// Ring: alpha(A1)→beta, beta(B2)→gamma, gamma(C3)→delta, delta(D4)→alpha
	const wantPackets = 5
	const timeout = 20 * time.Second

	type expectation struct {
		receiver int
		sender   int
		name     string
	}
	expectations := []expectation{
		{1, 0, "alpha→beta"},
		{2, 1, "beta→gamma"},
		{3, 2, "gamma→delta"},
		{0, 3, "delta→alpha"},
	}

	var wg sync.WaitGroup
	results := make([]int, 4)
	for i, exp := range expectations {
		wg.Add(1)
		go func(idx int, e expectation) {
			defer wg.Done()
			results[idx] = clients[e.receiver].waitForPayloads(markers[e.sender], wantPackets, timeout)
		}(i, exp)
	}
	wg.Wait()

	allPassed := true
	for i, exp := range expectations {
		if results[i] >= wantPackets {
			log.Printf("  ✓ %s: %d packets received", exp.name, results[i])
		} else {
			log.Printf("  ✗ %s: only %d/%d packets received", exp.name, results[i], wantPackets)
			allPassed = false
		}
	}

	// Validate non-silence
	for i, c := range clients {
		silenceCount := 0
		totalChecked := 0
		drain:
		for {
			select {
			case p := <-c.received:
				totalChecked++
				if bytes.Equal(p, make([]byte, len(p))) {
					silenceCount++
				}
			default:
				break drain
			}
		}
		if totalChecked > 0 && silenceCount == totalChecked {
			log.Printf("  ✗ %s: all %d received packets were silence", roles[i], totalChecked)
			allPassed = false
		}
	}

	if !allPassed {
		return fmt.Errorf("ring forwarding validation failed")
	}

	// Clean up session
	apiCall("DELETE", "/api/sessions/"+sessionID, sessionToken, nil)

	return nil
}
