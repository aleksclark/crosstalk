package abc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
	"nhooyr.io/websocket"
)

type signalingMessage struct {
	Type      string          `json:"type"`
	SDP       string          `json:"sdp,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
}

func signalingURL(serverURL, token string) (string, error) {
	if serverURL == "" {
		return "", fmt.Errorf("abc: empty server URL")
	}
	base := strings.TrimRight(serverURL, "/")
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("abc: parse server URL: %w", err)
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	case "wss", "ws":
	default:
		return "", fmt.Errorf("abc: unsupported server URL scheme %q", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/ws/signaling"
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (s *Session) sendSignaling(ctx context.Context, msg signalingMessage) error {
	s.mu.Lock()
	ws := s.wsConn
	s.mu.Unlock()
	if ws == nil {
		return fmt.Errorf("abc: websocket not connected")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("abc: marshal signaling: %w", err)
	}
	return ws.Write(ctx, websocket.MessageText, data)
}

func (s *Session) readSignalingLoop(ctx context.Context) {
	s.mu.Lock()
	ws := s.wsConn
	pc := s.peerConn
	s.mu.Unlock()
	if ws == nil || pc == nil {
		return
	}

	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			s.fail(fmt.Errorf("signaling: %w", err), "signaling closed")
			return
		}

		var msg signalingMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			s.logger.Warn("abc: ignoring unparseable signaling message")
			continue
		}

		switch msg.Type {
		case "answer":
			answer := webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: msg.SDP}
			if err := pc.SetRemoteDescription(answer); err != nil {
				s.fail(fmt.Errorf("setting remote description: %w", err), "remote description failed")
				return
			}
			s.captureNegotiatedCodec()
			s.logger.Info("abc: received SDP answer")
			select {
			case s.answerCh <- struct{}{}:
			default:
			}

		case "offer":
			s.logger.Info("abc: received renegotiation offer")
			offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: msg.SDP}
			if err := pc.SetRemoteDescription(offer); err != nil {
				s.logger.Error("abc: failed to set renegotiation offer", "error", err)
				continue
			}
			answer, err := pc.CreateAnswer(nil)
			if err != nil {
				s.logger.Error("abc: failed to create renegotiation answer", "error", err)
				continue
			}
			if err := pc.SetLocalDescription(answer); err != nil {
				s.logger.Error("abc: failed to set local description for renegotiation", "error", err)
				continue
			}
			if err := s.sendSignaling(ctx, signalingMessage{Type: "answer", SDP: answer.SDP}); err != nil {
				s.logger.Error("abc: failed to send renegotiation answer", "error", err)
				continue
			}
			s.captureNegotiatedCodec()
			s.logger.Info("abc: sent renegotiation answer")

		case "ice", "candidate":
			if len(msg.Candidate) == 0 || string(msg.Candidate) == "null" {
				continue
			}
			var candidateInit webrtc.ICECandidateInit
			if err := json.Unmarshal(msg.Candidate, &candidateInit); err != nil {
				var candidateStr string
				if err2 := json.Unmarshal(msg.Candidate, &candidateStr); err2 != nil {
					s.logger.Warn("abc: failed to parse ICE candidate")
					continue
				}
				candidateInit = webrtc.ICECandidateInit{Candidate: candidateStr}
			}
			if err := pc.AddICECandidate(candidateInit); err != nil {
				s.logger.Warn("abc: failed to add ICE candidate", "error", err)
			}

		default:
			s.logger.Warn("abc: unknown signaling message type", "type", msg.Type)
		}
	}
}

func newPeerAPI(disableMDNS bool) *webrtc.API {
	if !disableMDNS {
		return webrtc.NewAPI()
	}
	var se webrtc.SettingEngine
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	return webrtc.NewAPI(webrtc.WithSettingEngine(se))
}

func iceServers(cfg Config) []webrtc.ICEServer {
	if cfg.ICEServers != nil {
		return cfg.ICEServers
	}
	return []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}}
}
