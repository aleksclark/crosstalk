package pion

import (
	"log/slog"

	"github.com/pion/webrtc/v4"
)

// createControlChannel creates the server-owned "control" data channel on the
// peer connection. The channel is created with ordered delivery. By default it
// echoes any received message back to the sender — this behaviour can be
// overridden with [PeerConn.OnControlMessage].
func (c *PeerConn) createControlChannel() error {
	dc, err := c.pc.CreateDataChannel("control", &webrtc.DataChannelInit{
		Ordered: boolPtr(true),
	})
	if err != nil {
		return err
	}

	c.control = dc

	// Default handler: echo received messages back.
	dc.OnOpen(func() {
		slog.Debug("control data channel opened", "peer", c.ID)
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if err := dc.Send(msg.Data); err != nil {
			slog.Error("control echo send failed", "peer", c.ID, "err", err)
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
