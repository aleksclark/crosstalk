package pion

import (
	"log/slog"

	"github.com/pion/webrtc/v4"
)

// createControlChannel creates the server-owned "control" data channel on the
// peer connection. The channel is created with ordered delivery. It also
// registers an OnDataChannel handler to capture control data channels created
// by the remote client — this is needed because when both sides create a DC
// with the same label they get two independent channels.
func (c *PeerConn) createControlChannel() error {
	dc, err := c.pc.CreateDataChannel("control", &webrtc.DataChannelInit{
		Ordered: boolPtr(true),
	})
	if err != nil {
		return err
	}

	c.control = dc

	dc.OnOpen(func() {
		slog.Debug("control data channel opened (server-created)", "peer", c.ID)
	})

	// Listen for data channels created by the remote client. If the client
	// creates its own "control" DC, store it and wire its OnMessage to the
	// same dispatcher so messages from the client-created DC are handled.
	c.pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != "control" {
			slog.Debug("ignoring non-control data channel from client",
				"peer", c.ID, "label", dc.Label())
			return
		}

		slog.Debug("received client-created control data channel", "peer", c.ID)
		c.mu.Lock()
		c.clientControl = dc
		cb := c.onControlMessage
		c.mu.Unlock()

		dc.OnOpen(func() {
			slog.Debug("control data channel opened (client-created)", "peer", c.ID)
		})

		// If the control handler has already been installed, wire up the
		// client-created DC's OnMessage to the same dispatcher.
		if cb != nil {
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				cb(msg.Data)
			})
			c.control.OnMessage(func(webrtc.DataChannelMessage) {})
		}
	})

	return nil
}

// OnControlMessage replaces the default echo handler on the "control" data
// channel with a custom message handler.
func (c *PeerConn) OnControlMessage(f func(msg []byte)) {
	c.control.OnMessage(func(msg webrtc.DataChannelMessage) {
		f(msg.Data)
	})
}

// SendControl sends a binary message on the control data channel.
func (c *PeerConn) SendControl(data []byte) error {
	return c.control.Send(data)
}
