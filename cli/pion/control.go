package pion

import (
	"fmt"
	"log/slog"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/cli"
	crosstalkv1 "github.com/aleksclark/crosstalk/proto/gen/go/crosstalk/v1"
	"google.golang.org/protobuf/proto"
)

// SendHello sends a Hello message on the control data channel with the client's capabilities.
func (c *Connection) SendHello(sources []crosstalk.Source, sinks []crosstalk.Sink, codecs []crosstalk.Codec) error {
	hello := &crosstalkv1.Hello{
		Sources: make([]*crosstalkv1.SourceInfo, len(sources)),
		Sinks:   make([]*crosstalkv1.SinkInfo, len(sinks)),
		Codecs:  make([]*crosstalkv1.CodecInfo, len(codecs)),
	}

	for i, s := range sources {
		hello.Sources[i] = &crosstalkv1.SourceInfo{Name: s.Name, Type: s.Type}
	}
	for i, s := range sinks {
		hello.Sinks[i] = &crosstalkv1.SinkInfo{Name: s.Name, Type: s.Type}
	}
	for i, co := range codecs {
		hello.Codecs[i] = &crosstalkv1.CodecInfo{Name: co.Name, MediaType: co.MediaType}
	}

	msg := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_Hello{
			Hello: hello,
		},
	}

	slog.Info("sending Hello message",
		"sources", len(sources),
		"sinks", len(sinks),
		"codecs", len(codecs),
	)

	return c.SendControlMessage(msg)
}

// SendClientStatus sends a ClientStatus update on the control data channel.
func (c *Connection) SendClientStatus(state crosstalkv1.ClientState, sources []crosstalk.Source, sinks []crosstalk.Sink, codecs []crosstalk.Codec) error {
	status := &crosstalkv1.ClientStatus{
		State:   state,
		Sources: make([]*crosstalkv1.SourceInfo, len(sources)),
		Sinks:   make([]*crosstalkv1.SinkInfo, len(sinks)),
		Codecs:  make([]*crosstalkv1.CodecInfo, len(codecs)),
	}

	for i, s := range sources {
		status.Sources[i] = &crosstalkv1.SourceInfo{Name: s.Name, Type: s.Type}
	}
	for i, s := range sinks {
		status.Sinks[i] = &crosstalkv1.SinkInfo{Name: s.Name, Type: s.Type}
	}
	for i, co := range codecs {
		status.Codecs[i] = &crosstalkv1.CodecInfo{Name: co.Name, MediaType: co.MediaType}
	}

	msg := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_ClientStatus{
			ClientStatus: status,
		},
	}

	slog.Info("sending ClientStatus",
		"state", state,
		"sources", len(sources),
		"sinks", len(sinks),
	)

	return c.SendControlMessage(msg)
}

// SendJoinSession sends a JoinSession request on the control data channel.
func (c *Connection) SendJoinSession(sessionID, role string) error {
	msg := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_JoinSession{
			JoinSession: &crosstalkv1.JoinSession{
				SessionId: sessionID,
				Role:      role,
			},
		},
	}

	slog.Info("sending JoinSession", "session_id", sessionID, "role", role)
	return c.SendControlMessage(msg)
}

// SendLogEntry sends a LogEntry message on the control data channel.
func (c *Connection) SendLogEntry(severity crosstalkv1.LogSeverity, source, message string) error {
	msg := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_LogEntry{
			LogEntry: &crosstalkv1.LogEntry{
				Timestamp: time.Now().UnixMilli(),
				Severity:  severity,
				Source:    source,
				Message:   message,
			},
		},
	}

	return c.SendControlMessage(msg)
}

// SendChannelStatus sends a ChannelStatus report on the control data channel.
func (c *Connection) SendChannelStatus(channelID string, state crosstalkv1.ChannelState, errorMsg string, bytesTransferred uint64) error {
	msg := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_ChannelStatus{
			ChannelStatus: &crosstalkv1.ChannelStatus{
				ChannelId:        channelID,
				State:            state,
				ErrorMessage:     errorMsg,
				BytesTransferred: bytesTransferred,
			},
		},
	}

	return c.SendControlMessage(msg)
}

// SendControlMessage marshals a ControlMessage to protobuf and sends it on the control data channel.
func (c *Connection) SendControlMessage(msg *crosstalkv1.ControlMessage) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling control message: %w", err)
	}
	return c.SendControl(data)
}

// ParseControlMessage unmarshals raw bytes into a protobuf ControlMessage.
func ParseControlMessage(data []byte) (*crosstalkv1.ControlMessage, error) {
	var msg crosstalkv1.ControlMessage
	if err := proto.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("parsing control message: %w", err)
	}
	return &msg, nil
}
