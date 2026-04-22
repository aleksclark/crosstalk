package pion

import (
	"encoding/json"
	"fmt"
	"log/slog"

	crosstalk "github.com/aleksclark/crosstalk/cli"
)

// ControlMessageType identifies the type of control channel message.
type ControlMessageType string

const (
	ControlTypeHello         ControlMessageType = "hello"
	ControlTypeClientStatus  ControlMessageType = "client_status"
	ControlTypeWelcome       ControlMessageType = "welcome"
)

// ControlMessage is the envelope for all control data channel messages.
// This uses JSON encoding matching the protobuf ControlMessage schema.
type ControlMessage struct {
	Type         ControlMessageType `json:"type"`
	Hello        *HelloMessage      `json:"hello,omitempty"`
	ClientStatus *ClientStatusMsg   `json:"client_status,omitempty"`
	Welcome      *WelcomeMessage    `json:"welcome,omitempty"`
}

// HelloMessage is sent by the client immediately after the control channel opens.
type HelloMessage struct {
	Sources []SourceInfo `json:"sources"`
	Sinks   []SinkInfo   `json:"sinks"`
	Codecs  []CodecInfo  `json:"codecs"`
}

// SourceInfo reports an available audio/video source.
type SourceInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// SinkInfo reports an available audio/video sink.
type SinkInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// CodecInfo reports a supported codec.
type CodecInfo struct {
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
}

// ClientStatusMsg reports the client's current state and capabilities.
type ClientStatusMsg struct {
	State   string       `json:"state"` // "READY", "BUSY", "ERROR"
	Sources []SourceInfo `json:"sources"`
	Sinks   []SinkInfo   `json:"sinks"`
	Codecs  []CodecInfo  `json:"codecs"`
}

// WelcomeMessage is sent by the server after the control channel opens.
type WelcomeMessage struct {
	ClientID      string `json:"client_id"`
	ServerVersion string `json:"server_version"`
}

// SendHello sends a Hello message on the control data channel with the client's capabilities.
func (c *Connection) SendHello(sources []crosstalk.Source, sinks []crosstalk.Sink, codecs []crosstalk.Codec) error {
	hello := &HelloMessage{
		Sources: make([]SourceInfo, len(sources)),
		Sinks:   make([]SinkInfo, len(sinks)),
		Codecs:  make([]CodecInfo, len(codecs)),
	}

	for i, s := range sources {
		hello.Sources[i] = SourceInfo{Name: s.Name, Type: s.Type}
	}
	for i, s := range sinks {
		hello.Sinks[i] = SinkInfo{Name: s.Name, Type: s.Type}
	}
	for i, c := range codecs {
		hello.Codecs[i] = CodecInfo{Name: c.Name, MediaType: c.MediaType}
	}

	msg := ControlMessage{
		Type:  ControlTypeHello,
		Hello: hello,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling Hello message: %w", err)
	}

	slog.Info("sending Hello message",
		"sources", len(sources),
		"sinks", len(sinks),
		"codecs", len(codecs),
	)

	return c.SendControl(data)
}

// SendClientStatus sends a ClientStatus update on the control data channel.
func (c *Connection) SendClientStatus(state string, sources []crosstalk.Source, sinks []crosstalk.Sink, codecs []crosstalk.Codec) error {
	status := &ClientStatusMsg{
		State:   state,
		Sources: make([]SourceInfo, len(sources)),
		Sinks:   make([]SinkInfo, len(sinks)),
		Codecs:  make([]CodecInfo, len(codecs)),
	}

	for i, s := range sources {
		status.Sources[i] = SourceInfo{Name: s.Name, Type: s.Type}
	}
	for i, s := range sinks {
		status.Sinks[i] = SinkInfo{Name: s.Name, Type: s.Type}
	}
	for i, co := range codecs {
		status.Codecs[i] = CodecInfo{Name: co.Name, MediaType: co.MediaType}
	}

	msg := ControlMessage{
		Type:         ControlTypeClientStatus,
		ClientStatus: status,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling ClientStatus message: %w", err)
	}

	slog.Info("sending ClientStatus",
		"state", state,
		"sources", len(sources),
		"sinks", len(sinks),
	)

	return c.SendControl(data)
}

// ParseControlMessage parses a control channel message from JSON bytes.
func ParseControlMessage(data []byte) (*ControlMessage, error) {
	var msg ControlMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("parsing control message: %w", err)
	}
	return &msg, nil
}
