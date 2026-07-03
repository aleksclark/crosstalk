// Package protov2 provides v2 control protocol message types for the ABC client.
// These types are wire-compatible with the server's proto/v2/control.proto.
// They are defined locally to avoid cross-module dependencies between cli and server.
//
// The server's v2 ControlMessage uses protobuf oneof with these field numbers:
//
//	1  = Hello (client → server)
//	2  = SourceStatus (client → server)
//	10 = Welcome (server → client)
//	11 = MixUpdate (server → client)
//	12 = RestartCommand (server → client)
//	13 = SessionAssignment (server → client)
//	20 = LogEntry (bidirectional)
//	21 = PingPong (bidirectional)
package protov2

import (
	"encoding/binary"
	"fmt"
	"math"
)

// PayloadType identifies which payload is in the ControlMessage.
type PayloadType int

const (
	PayloadUnknown           PayloadType = 0
	PayloadHello             PayloadType = 1
	PayloadSourceStatus      PayloadType = 2
	PayloadWelcome           PayloadType = 10
	PayloadMixUpdate         PayloadType = 11
	PayloadRestart           PayloadType = 12
	PayloadSessionAssignment PayloadType = 13
	PayloadLogEntry          PayloadType = 20
	PayloadPing              PayloadType = 21
)

// ControlMessageV2 represents a parsed v2 control message.
type ControlMessageV2 struct {
	Type              PayloadType
	Welcome           *WelcomeV2
	Restart           *RestartCommandV2
	SessionAssignment *SessionAssignmentV2
}

// WelcomeV2 matches the server's v2 Welcome message.
type WelcomeV2 struct {
	PeerID            string
	ServerVersion     string
	AssignedSessionID string
}

// RestartCommandV2 matches the server's v2 RestartCommand message.
type RestartCommandV2 struct {
	Reason string
}

// SessionAssignmentV2 matches the server's v2 SessionAssignment message.
type SessionAssignmentV2 struct {
	SessionID string
	Role      string
}

// ParseControlMessageV2 decodes raw protobuf bytes into a ControlMessageV2.
func ParseControlMessageV2(data []byte) (*ControlMessageV2, error) {
	msg := &ControlMessageV2{}
	remaining := data

	for len(remaining) > 0 {
		fieldNum, wireType, n := consumeTag(remaining)
		if n < 0 {
			return nil, fmt.Errorf("protov2: invalid tag")
		}
		remaining = remaining[n:]

		if wireType != 2 {
			skip := skipField(wireType, remaining)
			if skip < 0 {
				return nil, fmt.Errorf("protov2: cannot skip field %d", fieldNum)
			}
			remaining = remaining[skip:]
			continue
		}

		valueLen, lenBytes := consumeVarint(remaining)
		if lenBytes < 0 {
			return nil, fmt.Errorf("protov2: invalid length")
		}
		remaining = remaining[lenBytes:]
		if int(valueLen) > len(remaining) {
			return nil, fmt.Errorf("protov2: truncated message")
		}
		value := remaining[:valueLen]
		remaining = remaining[valueLen:]

		msg.Type = PayloadType(fieldNum)
		switch PayloadType(fieldNum) {
		case PayloadWelcome:
			w, err := parseWelcome(value)
			if err != nil {
				return nil, err
			}
			msg.Welcome = w
		case PayloadRestart:
			r, err := parseRestart(value)
			if err != nil {
				return nil, err
			}
			msg.Restart = r
		case PayloadSessionAssignment:
			sa, err := parseSessionAssignment(value)
			if err != nil {
				return nil, err
			}
			msg.SessionAssignment = sa
		}
	}

	return msg, nil
}

func parseWelcome(data []byte) (*WelcomeV2, error) {
	w := &WelcomeV2{}
	remaining := data
	for len(remaining) > 0 {
		fieldNum, wireType, n := consumeTag(remaining)
		if n < 0 {
			return nil, fmt.Errorf("protov2: invalid welcome tag")
		}
		remaining = remaining[n:]

		if wireType == 2 {
			valueLen, lenBytes := consumeVarint(remaining)
			if lenBytes < 0 {
				return nil, fmt.Errorf("protov2: invalid welcome length")
			}
			remaining = remaining[lenBytes:]
			if int(valueLen) > len(remaining) {
				return nil, fmt.Errorf("protov2: truncated welcome field")
			}
			value := remaining[:valueLen]
			remaining = remaining[valueLen:]

			switch fieldNum {
			case 1:
				w.PeerID = string(value)
			case 2:
				w.ServerVersion = string(value)
			case 3:
				w.AssignedSessionID = string(value)
			}
		} else {
			skip := skipField(wireType, remaining)
			if skip < 0 {
				return nil, fmt.Errorf("protov2: cannot skip welcome field")
			}
			remaining = remaining[skip:]
		}
	}
	return w, nil
}

func parseRestart(data []byte) (*RestartCommandV2, error) {
	r := &RestartCommandV2{}
	remaining := data
	for len(remaining) > 0 {
		fieldNum, wireType, n := consumeTag(remaining)
		if n < 0 {
			return nil, fmt.Errorf("protov2: invalid restart tag")
		}
		remaining = remaining[n:]

		if wireType == 2 {
			valueLen, lenBytes := consumeVarint(remaining)
			if lenBytes < 0 {
				return nil, fmt.Errorf("protov2: invalid restart length")
			}
			remaining = remaining[lenBytes:]
			if int(valueLen) > len(remaining) {
				return nil, fmt.Errorf("protov2: truncated restart field")
			}
			value := remaining[:valueLen]
			remaining = remaining[valueLen:]

			if fieldNum == 1 {
				r.Reason = string(value)
			}
		} else {
			skip := skipField(wireType, remaining)
			if skip < 0 {
				return nil, fmt.Errorf("protov2: cannot skip restart field")
			}
			remaining = remaining[skip:]
		}
	}
	return r, nil
}

func parseSessionAssignment(data []byte) (*SessionAssignmentV2, error) {
	sa := &SessionAssignmentV2{}
	remaining := data
	for len(remaining) > 0 {
		fieldNum, wireType, n := consumeTag(remaining)
		if n < 0 {
			return nil, fmt.Errorf("protov2: invalid session_assignment tag")
		}
		remaining = remaining[n:]

		if wireType == 2 {
			valueLen, lenBytes := consumeVarint(remaining)
			if lenBytes < 0 {
				return nil, fmt.Errorf("protov2: invalid session_assignment length")
			}
			remaining = remaining[lenBytes:]
			if int(valueLen) > len(remaining) {
				return nil, fmt.Errorf("protov2: truncated session_assignment field")
			}
			value := remaining[:valueLen]
			remaining = remaining[valueLen:]

			switch fieldNum {
			case 1:
				sa.SessionID = string(value)
			case 2:
				sa.Role = string(value)
			}
		} else {
			skip := skipField(wireType, remaining)
			if skip < 0 {
				return nil, fmt.Errorf("protov2: cannot skip session_assignment field")
			}
			remaining = remaining[skip:]
		}
	}
	return sa, nil
}

// --- Wire encoding helpers ---

// BuildHelloV2 creates a v2 Hello control message in wire format.
// v2 Hello: client_type=field1(string), client_name=field2(string)
func BuildHelloV2(clientType, clientName string) []byte {
	var hello []byte
	if clientType != "" {
		hello = appendStringField(hello, 1, clientType)
	}
	if clientName != "" {
		hello = appendStringField(hello, 2, clientName)
	}

	// Wrap in ControlMessage: hello = field 1 (bytes).
	var msg []byte
	msg = appendBytesField(msg, 1, hello)
	return msg
}

// BuildSourceStatusV2 creates a v2 SourceStatus control message.
// SourceStatus: source_name=1(string), active=2(bool/varint), peak_level=3(float32/fixed32)
func BuildSourceStatusV2(sourceName string, active bool, peakLevel float32) []byte {
	var status []byte
	if sourceName != "" {
		status = appendStringField(status, 1, sourceName)
	}
	if active {
		status = appendVarintField(status, 2, 1)
	}
	if peakLevel != 0 {
		status = appendFixed32Field(status, 3, peakLevel)
	}

	// Wrap in ControlMessage: source_status = field 2 (bytes).
	var msg []byte
	msg = appendBytesField(msg, 2, status)
	return msg
}

// --- Low-level protobuf wire format helpers ---

func consumeTag(data []byte) (fieldNum int, wireType int, n int) {
	v, n := consumeVarint(data)
	if n < 0 {
		return 0, 0, -1
	}
	return int(v >> 3), int(v & 0x7), n
}

func consumeVarint(data []byte) (uint64, int) {
	var val uint64
	for i := 0; i < len(data) && i < 10; i++ {
		b := data[i]
		val |= uint64(b&0x7f) << (7 * i)
		if b < 0x80 {
			return val, i + 1
		}
	}
	return 0, -1
}

func skipField(wireType int, data []byte) int {
	switch wireType {
	case 0: // varint
		_, n := consumeVarint(data)
		return n
	case 1: // 64-bit
		if len(data) < 8 {
			return -1
		}
		return 8
	case 2: // length-delimited
		valueLen, n := consumeVarint(data)
		if n < 0 {
			return -1
		}
		total := n + int(valueLen)
		if total > len(data) {
			return -1
		}
		return total
	case 5: // 32-bit
		if len(data) < 4 {
			return -1
		}
		return 4
	default:
		return -1
	}
}

func appendStringField(data []byte, fieldNum int, s string) []byte {
	return appendBytesField(data, fieldNum, []byte(s))
}

func appendBytesField(data []byte, fieldNum int, value []byte) []byte {
	tag := uint64(fieldNum<<3) | 2
	data = appendRawVarint(data, tag)
	data = appendRawVarint(data, uint64(len(value)))
	data = append(data, value...)
	return data
}

func appendVarintField(data []byte, fieldNum int, value uint64) []byte {
	tag := uint64(fieldNum << 3)
	data = appendRawVarint(data, tag)
	data = appendRawVarint(data, value)
	return data
}

func appendFixed32Field(data []byte, fieldNum int, value float32) []byte {
	tag := uint64(fieldNum<<3) | 5
	data = appendRawVarint(data, tag)
	bits := math.Float32bits(value)
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, bits)
	data = append(data, buf...)
	return data
}

func appendRawVarint(data []byte, v uint64) []byte {
	for v >= 0x80 {
		data = append(data, byte(v)|0x80)
		v >>= 7
	}
	data = append(data, byte(v))
	return data
}
