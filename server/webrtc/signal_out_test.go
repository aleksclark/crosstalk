package webrtc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

// TestSignalOut_AnswerPrecedesBufferedICE proves the hold/flush contract without
// relying on Pion timing: ICE enqueued under hold is written only after SDP.
func TestSignalOut_AnswerPrecedesBufferedICE(t *testing.T) {
	upgraded := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		upgraded <- c
		// Keep the handler alive until the client disconnects.
		_, _, _ = c.Read(r.Context())
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	client, _, err := websocket.Dial(ctx, "ws"+srv.URL[len("http"):], nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close(websocket.StatusNormalClosure, "") })

	var serverConn *websocket.Conn
	select {
	case serverConn = <-upgraded:
	case <-ctx.Done():
		t.Fatal("server upgrade timeout")
	}

	peer := &PeerConn{ID: "test-peer", events: NewEventRing(64)}
	out := newSignalOut(ctx, serverConn, peer)

	out.HoldICE()
	// Simulate Pion racing ICE before the answer write completes.
	out.WriteICE(webrtc.ICECandidateInit{Candidate: "candidate:1 1 udp 1 127.0.0.1 9 typ host"})
	out.WriteICE(webrtc.ICECandidateInit{Candidate: "candidate:2 1 udp 1 127.0.0.1 10 typ host"})
	require.NoError(t, out.WriteSDP("answer", "v=0\r\n"))

	// Client must observe answer then the two candidates, in order.
	got := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		_, data, err := client.Read(ctx)
		require.NoError(t, err)
		var msg SignalMessage
		require.NoError(t, json.Unmarshal(data, &msg))
		got = append(got, msg.Type)
	}
	require.Equal(t, []string{"answer", "candidate", "candidate"}, got)

	// No hold: ICE writes immediately.
	out.WriteICE(webrtc.ICECandidateInit{Candidate: "candidate:3 1 udp 1 127.0.0.1 11 typ host"})
	_, data, err := client.Read(ctx)
	require.NoError(t, err)
	var msg SignalMessage
	require.NoError(t, json.Unmarshal(data, &msg))
	require.Equal(t, "candidate", msg.Type)

	// Drain server side so Accept handler can exit cleanly.
	_ = client.Close(websocket.StatusNormalClosure, "")
	_ = serverConn.Close(websocket.StatusNormalClosure, "")
}
