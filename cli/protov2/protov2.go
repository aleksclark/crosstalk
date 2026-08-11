// Package protov2 provides v2 control protocol message types for the ABC client.
// These types are wire-compatible with the server's proto/v2/control.proto.
// They are defined locally to avoid cross-module dependencies between cli and server.
//
// The server's v2 ControlMessage uses protobuf oneof with these field numbers:
//
//	1  = Hello (client → server)
//	2  = SourceStatus (client → server)
//	3  = AudioControlReport (client → server)
//	10 = Welcome (server → client)
//	11 = MixUpdate (server → client)
//	12 = RestartCommand (server → client)
//	13 = SessionAssignment (server → client)
//	14 = AudioControlCommand (server → client)
//	20 = LogEntry (bidirectional)
//	21 = PingPong (bidirectional)
package protov2

import (
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf8"
)

// Wire/protocol limits (enforced before allocation where practical).
const (
	MaxControlMessageBytes = 16 * 1024
	MaxDevices             = 8
	MaxStringBytes         = 128
	MaxErrorDetailBytes    = 256
	MaxCommandIDBytes      = 128
)

// PayloadType identifies which payload is in the ControlMessage.
type PayloadType int

const (
	PayloadUnknown             PayloadType = 0
	PayloadHello               PayloadType = 1
	PayloadSourceStatus        PayloadType = 2
	PayloadAudioControlReport  PayloadType = 3
	PayloadWelcome             PayloadType = 10
	PayloadMixUpdate           PayloadType = 11
	PayloadRestart             PayloadType = 12
	PayloadSessionAssignment   PayloadType = 13
	PayloadAudioControlCommand PayloadType = 14
	PayloadLogEntry            PayloadType = 20
	PayloadPing                PayloadType = 21
)

// AudioApplyState matches the server's AudioApplyState enum.
type AudioApplyState int

const (
	AudioApplyUnspecified    AudioApplyState = 0
	AudioApplyApplied        AudioApplyState = 1
	AudioApplyUnsupported    AudioApplyState = 2
	AudioApplyError          AudioApplyState = 3
	AudioApplyDeviceMismatch AudioApplyState = 4
	AudioApplyStaleRevision  AudioApplyState = 5
)

// ControlMessageV2 represents a parsed v2 control message.
type ControlMessageV2 struct {
	Type                PayloadType
	Welcome             *WelcomeV2
	Restart             *RestartCommandV2
	SessionAssignment   *SessionAssignmentV2
	AudioControlCommand *AudioControlCommand
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

// AudioControlCommand is an absolute desired USB mixer state (server → client).
type AudioControlCommand struct {
	CommandID        string
	DesiredRevision  uint64
	Output           *AudioOutputDesired
	Input            *AudioInputDesired
}

// AudioOutputDesired is the absolute desired playback mixer state.
type AudioOutputDesired struct {
	DeviceUID      string
	VolumePercent  *uint32 // presence distinguishes 0 from unset
	Muted          *bool
}

// AudioInputDesired is the absolute desired capture mixer state.
type AudioInputDesired struct {
	DeviceUID   string
	GainPercent *uint32
}

// AudioControlReport is inventory and/or apply result (client → server).
type AudioControlReport struct {
	CommandID       string
	DesiredRevision uint64
	Devices         []AudioDeviceCapability
	Output          *AudioOutputObserved
	Input           *AudioInputObserved
	ErrorCode       string
	ErrorDetail     string
}

// AudioDeviceCapability describes one discovered audio endpoint.
type AudioDeviceCapability struct {
	DeviceUID          string
	Direction          string
	Backend            string
	VendorID           string
	ProductID          string
	Serial             string
	Path               string
	ALSACardID         string
	CardName           string
	PCMRoute           string
	SupportsVolume     bool
	SupportsMute       bool
	SupportsGain       bool
	SupportsAGCDisable bool
	VolumeMinDB        *int32
	VolumeMaxDB        *int32
	VolumeStepDB       *int32
	GainMinDB          *int32
	GainMaxDB          *int32
	GainStepDB         *int32
}

// AudioOutputObserved is the effective playback mixer readback.
type AudioOutputObserved struct {
	DeviceUID     string
	VolumePercent *uint32
	Muted         *bool
	VolumeState   AudioApplyState
	MuteState     AudioApplyState
}

// AudioInputObserved is the effective capture mixer readback.
type AudioInputObserved struct {
	DeviceUID   string
	GainPercent *uint32
	GainState   AudioApplyState
}

// ParseControlMessageV2 decodes raw protobuf bytes into a ControlMessageV2.
func ParseControlMessageV2(data []byte) (*ControlMessageV2, error) {
	if len(data) > MaxControlMessageBytes {
		return nil, fmt.Errorf("protov2: message exceeds %d bytes", MaxControlMessageBytes)
	}
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
		case PayloadAudioControlCommand:
			cmd, err := parseAudioControlCommand(value)
			if err != nil {
				return nil, err
			}
			msg.AudioControlCommand = cmd
		default:
			// Skip unknown length-delimited payloads (already consumed).
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
			value, rest, err := consumeBytes(remaining)
			if err != nil {
				return nil, fmt.Errorf("protov2: welcome field: %w", err)
			}
			remaining = rest
			switch fieldNum {
			case 1:
				w.PeerID = clampString(string(value), MaxStringBytes)
			case 2:
				w.ServerVersion = clampString(string(value), MaxStringBytes)
			case 3:
				w.AssignedSessionID = clampString(string(value), MaxStringBytes)
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
			value, rest, err := consumeBytes(remaining)
			if err != nil {
				return nil, fmt.Errorf("protov2: restart field: %w", err)
			}
			remaining = rest
			if fieldNum == 1 {
				r.Reason = clampString(string(value), MaxStringBytes)
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
			value, rest, err := consumeBytes(remaining)
			if err != nil {
				return nil, fmt.Errorf("protov2: session_assignment field: %w", err)
			}
			remaining = rest
			switch fieldNum {
			case 1:
				sa.SessionID = clampString(string(value), MaxStringBytes)
			case 2:
				sa.Role = clampString(string(value), MaxStringBytes)
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

func parseAudioControlCommand(data []byte) (*AudioControlCommand, error) {
	cmd := &AudioControlCommand{}
	remaining := data
	for len(remaining) > 0 {
		fieldNum, wireType, n := consumeTag(remaining)
		if n < 0 {
			return nil, fmt.Errorf("protov2: invalid audio_control_command tag")
		}
		remaining = remaining[n:]

		switch wireType {
		case 0: // varint
			v, vn := consumeVarint(remaining)
			if vn < 0 {
				return nil, fmt.Errorf("protov2: invalid audio_control_command varint")
			}
			remaining = remaining[vn:]
			if fieldNum == 2 {
				cmd.DesiredRevision = v
			}
		case 2:
			value, rest, err := consumeBytes(remaining)
			if err != nil {
				return nil, fmt.Errorf("protov2: audio_control_command field: %w", err)
			}
			remaining = rest
			switch fieldNum {
			case 1:
				if len(value) > MaxCommandIDBytes {
					return nil, fmt.Errorf("protov2: command_id exceeds %d bytes", MaxCommandIDBytes)
				}
				cmd.CommandID = string(value)
			case 3:
				out, err := parseAudioOutputDesired(value)
				if err != nil {
					return nil, err
				}
				cmd.Output = out
			case 4:
				in, err := parseAudioInputDesired(value)
				if err != nil {
					return nil, err
				}
				cmd.Input = in
			}
		default:
			skip := skipField(wireType, remaining)
			if skip < 0 {
				return nil, fmt.Errorf("protov2: cannot skip audio_control_command field")
			}
			remaining = remaining[skip:]
		}
	}
	return cmd, nil
}

func parseAudioOutputDesired(data []byte) (*AudioOutputDesired, error) {
	out := &AudioOutputDesired{}
	remaining := data
	for len(remaining) > 0 {
		fieldNum, wireType, n := consumeTag(remaining)
		if n < 0 {
			return nil, fmt.Errorf("protov2: invalid audio_output_desired tag")
		}
		remaining = remaining[n:]
		switch wireType {
		case 0:
			v, vn := consumeVarint(remaining)
			if vn < 0 {
				return nil, fmt.Errorf("protov2: invalid audio_output_desired varint")
			}
			remaining = remaining[vn:]
			switch fieldNum {
			case 2:
				u := uint32(v)
				out.VolumePercent = &u
			case 3:
				b := v != 0
				out.Muted = &b
			}
		case 2:
			value, rest, err := consumeBytes(remaining)
			if err != nil {
				return nil, err
			}
			remaining = rest
			if fieldNum == 1 {
				if len(value) > MaxStringBytes {
					return nil, fmt.Errorf("protov2: device_uid exceeds %d bytes", MaxStringBytes)
				}
				out.DeviceUID = string(value)
			}
		default:
			skip := skipField(wireType, remaining)
			if skip < 0 {
				return nil, fmt.Errorf("protov2: cannot skip audio_output_desired field")
			}
			remaining = remaining[skip:]
		}
	}
	return out, nil
}

func parseAudioInputDesired(data []byte) (*AudioInputDesired, error) {
	in := &AudioInputDesired{}
	remaining := data
	for len(remaining) > 0 {
		fieldNum, wireType, n := consumeTag(remaining)
		if n < 0 {
			return nil, fmt.Errorf("protov2: invalid audio_input_desired tag")
		}
		remaining = remaining[n:]
		switch wireType {
		case 0:
			v, vn := consumeVarint(remaining)
			if vn < 0 {
				return nil, fmt.Errorf("protov2: invalid audio_input_desired varint")
			}
			remaining = remaining[vn:]
			if fieldNum == 2 {
				u := uint32(v)
				in.GainPercent = &u
			}
		case 2:
			value, rest, err := consumeBytes(remaining)
			if err != nil {
				return nil, err
			}
			remaining = rest
			if fieldNum == 1 {
				if len(value) > MaxStringBytes {
					return nil, fmt.Errorf("protov2: device_uid exceeds %d bytes", MaxStringBytes)
				}
				in.DeviceUID = string(value)
			}
		default:
			skip := skipField(wireType, remaining)
			if skip < 0 {
				return nil, fmt.Errorf("protov2: cannot skip audio_input_desired field")
			}
			remaining = remaining[skip:]
		}
	}
	return in, nil
}

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

// BuildAudioControlReport creates a v2 AudioControlReport control message.
// Enforces device/string/error limits before encoding.
func BuildAudioControlReport(r AudioControlReport) ([]byte, error) {
	if len(r.Devices) > MaxDevices {
		return nil, fmt.Errorf("protov2: devices exceeds %d", MaxDevices)
	}
	if len(r.CommandID) > MaxCommandIDBytes {
		return nil, fmt.Errorf("protov2: command_id exceeds %d bytes", MaxCommandIDBytes)
	}
	if len(r.ErrorCode) > MaxStringBytes {
		return nil, fmt.Errorf("protov2: error_code exceeds %d bytes", MaxStringBytes)
	}
	if len(r.ErrorDetail) > MaxErrorDetailBytes {
		return nil, fmt.Errorf("protov2: error_detail exceeds %d bytes", MaxErrorDetailBytes)
	}

	var inner []byte
	if r.CommandID != "" {
		inner = appendStringField(inner, 1, r.CommandID)
	}
	if r.DesiredRevision != 0 {
		inner = appendVarintField(inner, 2, r.DesiredRevision)
	}
	for _, d := range r.Devices {
		dev, err := encodeDeviceCapability(d)
		if err != nil {
			return nil, err
		}
		inner = appendBytesField(inner, 3, dev)
	}
	if r.Output != nil {
		inner = appendBytesField(inner, 4, encodeOutputObserved(*r.Output))
	}
	if r.Input != nil {
		inner = appendBytesField(inner, 5, encodeInputObserved(*r.Input))
	}
	if r.ErrorCode != "" {
		inner = appendStringField(inner, 6, r.ErrorCode)
	}
	if r.ErrorDetail != "" {
		inner = appendStringField(inner, 7, r.ErrorDetail)
	}

	var msg []byte
	msg = appendBytesField(msg, 3, inner) // audio_control_report = field 3
	if len(msg) > MaxControlMessageBytes {
		return nil, fmt.Errorf("protov2: encoded report exceeds %d bytes", MaxControlMessageBytes)
	}
	return msg, nil
}

func encodeDeviceCapability(d AudioDeviceCapability) ([]byte, error) {
	if len(d.DeviceUID) > MaxStringBytes ||
		len(d.Direction) > MaxStringBytes ||
		len(d.Backend) > MaxStringBytes ||
		len(d.VendorID) > MaxStringBytes ||
		len(d.ProductID) > MaxStringBytes ||
		len(d.Serial) > MaxStringBytes ||
		len(d.Path) > MaxStringBytes ||
		len(d.ALSACardID) > MaxStringBytes ||
		len(d.CardName) > MaxStringBytes ||
		len(d.PCMRoute) > MaxStringBytes {
		return nil, fmt.Errorf("protov2: device string exceeds %d bytes", MaxStringBytes)
	}
	var b []byte
	if d.DeviceUID != "" {
		b = appendStringField(b, 1, d.DeviceUID)
	}
	if d.Direction != "" {
		b = appendStringField(b, 2, d.Direction)
	}
	if d.Backend != "" {
		b = appendStringField(b, 3, d.Backend)
	}
	if d.VendorID != "" {
		b = appendStringField(b, 4, d.VendorID)
	}
	if d.ProductID != "" {
		b = appendStringField(b, 5, d.ProductID)
	}
	if d.Serial != "" {
		b = appendStringField(b, 6, d.Serial)
	}
	if d.Path != "" {
		b = appendStringField(b, 7, d.Path)
	}
	if d.ALSACardID != "" {
		b = appendStringField(b, 8, d.ALSACardID)
	}
	if d.CardName != "" {
		b = appendStringField(b, 9, d.CardName)
	}
	if d.PCMRoute != "" {
		b = appendStringField(b, 10, d.PCMRoute)
	}
	if d.SupportsVolume {
		b = appendVarintField(b, 11, 1)
	}
	if d.SupportsMute {
		b = appendVarintField(b, 12, 1)
	}
	if d.SupportsGain {
		b = appendVarintField(b, 13, 1)
	}
	if d.SupportsAGCDisable {
		b = appendVarintField(b, 14, 1)
	}
	if d.VolumeMinDB != nil {
		b = appendZigZag32Field(b, 15, *d.VolumeMinDB)
	}
	if d.VolumeMaxDB != nil {
		b = appendZigZag32Field(b, 16, *d.VolumeMaxDB)
	}
	if d.VolumeStepDB != nil {
		b = appendZigZag32Field(b, 17, *d.VolumeStepDB)
	}
	if d.GainMinDB != nil {
		b = appendZigZag32Field(b, 18, *d.GainMinDB)
	}
	if d.GainMaxDB != nil {
		b = appendZigZag32Field(b, 19, *d.GainMaxDB)
	}
	if d.GainStepDB != nil {
		b = appendZigZag32Field(b, 20, *d.GainStepDB)
	}
	return b, nil
}

func encodeOutputObserved(o AudioOutputObserved) []byte {
	var b []byte
	if o.DeviceUID != "" {
		b = appendStringField(b, 1, clampString(o.DeviceUID, MaxStringBytes))
	}
	if o.VolumePercent != nil {
		b = appendVarintField(b, 2, uint64(*o.VolumePercent))
	}
	if o.Muted != nil {
		if *o.Muted {
			b = appendVarintField(b, 3, 1)
		} else {
			b = appendVarintField(b, 3, 0)
		}
	}
	if o.VolumeState != AudioApplyUnspecified {
		b = appendVarintField(b, 4, uint64(o.VolumeState))
	}
	if o.MuteState != AudioApplyUnspecified {
		b = appendVarintField(b, 5, uint64(o.MuteState))
	}
	return b
}

func encodeInputObserved(o AudioInputObserved) []byte {
	var b []byte
	if o.DeviceUID != "" {
		b = appendStringField(b, 1, clampString(o.DeviceUID, MaxStringBytes))
	}
	if o.GainPercent != nil {
		b = appendVarintField(b, 2, uint64(*o.GainPercent))
	}
	if o.GainState != AudioApplyUnspecified {
		b = appendVarintField(b, 3, uint64(o.GainState))
	}
	return b
}

// --- Low-level protobuf wire format helpers ---

func consumeBytes(data []byte) (value []byte, rest []byte, err error) {
	valueLen, lenBytes := consumeVarint(data)
	if lenBytes < 0 {
		return nil, nil, fmt.Errorf("invalid length")
	}
	data = data[lenBytes:]
	if int(valueLen) > len(data) {
		return nil, nil, fmt.Errorf("truncated field")
	}
	return data[:valueLen], data[valueLen:], nil
}

func clampString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	// Truncate on rune boundary when possible.
	if !utf8.ValidString(s[:max]) {
		for max > 0 && !utf8.ValidString(s[:max]) {
			max--
		}
	}
	return s[:max]
}

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

func appendZigZag32Field(data []byte, fieldNum int, value int32) []byte {
	// proto3 int32 uses zigzag only for sint32; plain int32 is two's complement varint.
	// Our schema uses optional int32 → encode as signed varint (zigzag NOT used for int32).
	// google.protobuf encodes int32 as varint of the two's-complement bit pattern.
	u := uint64(uint32(value))
	if value < 0 {
		// Sign-extend to 64-bit varint form used by protobuf for negative int32.
		u = uint64(int64(value))
	}
	return appendVarintField(data, fieldNum, u)
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
