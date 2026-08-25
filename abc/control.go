package abc

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	controlv2 "github.com/aleksclark/crosstalk/abc/internal/controlv2"
)

const maxControlMessageBytes = 16 * 1024

func defaultCapabilities() []*controlv2.AudioCapability {
	out := make([]*controlv2.AudioCapability, 0, len(DefaultHelloCapabilities))
	for _, cap := range DefaultHelloCapabilities {
		out = append(out, &controlv2.AudioCapability{
			Codec:      cap.Codec,
			Channels:   cap.Channels,
			SampleRate: cap.SampleRate,
		})
	}
	return out
}

func encodeHello(clientType, clientName string, caps []AudioCapability) ([]byte, error) {
	var protoCaps []*controlv2.AudioCapability
	if caps == nil {
		protoCaps = defaultCapabilities()
	} else {
		protoCaps = make([]*controlv2.AudioCapability, 0, len(caps))
		for _, cap := range caps {
			protoCaps = append(protoCaps, &controlv2.AudioCapability{
				Codec:      cap.Codec,
				Channels:   cap.Channels,
				SampleRate: cap.SampleRate,
			})
		}
	}
	return proto.Marshal(&controlv2.ControlMessage{
		Payload: &controlv2.ControlMessage_Hello{
			Hello: &controlv2.Hello{
				ClientType:   clientType,
				ClientName:   clientName,
				Capabilities: protoCaps,
			},
		},
	})
}

// EncodeAudioControlReport marshals a mixer inventory/apply report for the
// reliable control channel. Callers that only have a send-bytes hook should
// use this rather than importing the generated codec.
func EncodeAudioControlReport(r AudioControlReport) ([]byte, error) {
	return encodeControlReport(r)
}

func encodeControlReport(r AudioControlReport) ([]byte, error) {
	msg := &controlv2.ControlMessage{
		Payload: &controlv2.ControlMessage_AudioControlReport{
			AudioControlReport: toProtoReport(r),
		},
	}
	return proto.Marshal(msg)
}

func decodeControlMessage(data []byte) (*ControlMessage, error) {
	if len(data) > maxControlMessageBytes {
		return nil, &ProtocolError{Reason: "control message exceeds 16KiB"}
	}
	var cm controlv2.ControlMessage
	if err := proto.Unmarshal(data, &cm); err != nil {
		return nil, &ProtocolError{Reason: "malformed control frame"}
	}
	out := &ControlMessage{}
	switch payload := cm.GetPayload().(type) {
	case *controlv2.ControlMessage_Welcome:
		if w := payload.Welcome; w != nil {
			out.Welcome = &Welcome{
				PeerID:            w.GetPeerId(),
				ServerVersion:     w.GetServerVersion(),
				AssignedSessionID: w.GetAssignedSessionId(),
			}
		}
	case *controlv2.ControlMessage_Restart:
		if r := payload.Restart; r != nil {
			out.Restart = &RestartCommand{Reason: r.GetReason()}
		}
	case *controlv2.ControlMessage_SessionAssignment:
		if a := payload.SessionAssignment; a != nil {
			out.SessionAssignment = &SessionAssignment{
				SessionID: a.GetSessionId(),
				Role:      a.GetRole(),
			}
		}
	case *controlv2.ControlMessage_AudioControlCommand:
		if c := payload.AudioControlCommand; c != nil {
			out.AudioControlCommand = fromProtoCommand(c)
		}
	case *controlv2.ControlMessage_AudioControlReport:
		if r := payload.AudioControlReport; r != nil {
			out.AudioControlReport = fromProtoReport(r)
		}
	case *controlv2.ControlMessage_Hello,
		*controlv2.ControlMessage_SourceStatus,
		*controlv2.ControlMessage_MixUpdate,
		*controlv2.ControlMessage_LogEntry,
		*controlv2.ControlMessage_Ping:
		return out, nil
	default:
		if cm.GetPayload() == nil {
			return nil, &ProtocolError{Reason: "empty control payload"}
		}
		return out, nil
	}
	return out, nil
}

func toProtoReport(r AudioControlReport) *controlv2.AudioControlReport {
	out := &controlv2.AudioControlReport{
		CommandId:       r.CommandID,
		DesiredRevision: r.DesiredRevision,
		ErrorCode:       r.ErrorCode,
		ErrorDetail:     r.ErrorDetail,
	}
	for _, d := range r.Devices {
		out.Devices = append(out.Devices, &controlv2.AudioDeviceCapability{
			DeviceUid:          d.DeviceUID,
			Direction:          d.Direction,
			Backend:            d.Backend,
			VendorId:           d.VendorID,
			ProductId:          d.ProductID,
			Serial:             d.Serial,
			Path:               d.Path,
			AlsaCardId:         d.CardID,
			CardName:           d.CardName,
			PcmRoute:           d.PCMRoute,
			SupportsVolume:     d.SupportsVolume,
			SupportsMute:       d.SupportsMute,
			SupportsGain:       d.SupportsGain,
			SupportsAgcDisable: d.SupportsAGCDisable,
			VolumeMinDb:        d.VolumeMinDB,
			VolumeMaxDb:        d.VolumeMaxDB,
			VolumeStepDb:       d.VolumeStepDB,
			GainMinDb:          d.GainMinDB,
			GainMaxDb:          d.GainMaxDB,
			GainStepDb:         d.GainStepDB,
		})
	}
	if r.Output != nil {
		out.Output = &controlv2.AudioOutputObserved{
			DeviceUid:     r.Output.DeviceUID,
			VolumePercent: r.Output.VolumePercent,
			Muted:         r.Output.Muted,
			VolumeState:   controlv2.AudioApplyState(r.Output.VolumeState),
			MuteState:     controlv2.AudioApplyState(r.Output.MuteState),
		}
	}
	if r.Input != nil {
		out.Input = &controlv2.AudioInputObserved{
			DeviceUid:   r.Input.DeviceUID,
			GainPercent: r.Input.GainPercent,
			GainState:   controlv2.AudioApplyState(r.Input.GainState),
		}
	}
	return out
}

func fromProtoCommand(c *controlv2.AudioControlCommand) *AudioControlCommand {
	out := &AudioControlCommand{
		CommandID:       c.GetCommandId(),
		DesiredRevision: c.GetDesiredRevision(),
	}
	if o := c.GetOutput(); o != nil {
		out.Output = &AudioOutputDesired{
			DeviceUID:     o.GetDeviceUid(),
			VolumePercent: o.VolumePercent,
			Muted:         o.Muted,
		}
	}
	if i := c.GetInput(); i != nil {
		out.Input = &AudioInputDesired{
			DeviceUID:   i.GetDeviceUid(),
			GainPercent: i.GainPercent,
		}
	}
	return out
}

func fromProtoReport(r *controlv2.AudioControlReport) *AudioControlReport {
	out := &AudioControlReport{
		CommandID:       r.GetCommandId(),
		DesiredRevision: r.GetDesiredRevision(),
		ErrorCode:       r.GetErrorCode(),
		ErrorDetail:     r.GetErrorDetail(),
	}
	for _, d := range r.GetDevices() {
		if d == nil {
			continue
		}
		out.Devices = append(out.Devices, AudioDeviceCapability{
			DeviceUID:          d.GetDeviceUid(),
			Direction:          d.GetDirection(),
			Backend:            d.GetBackend(),
			VendorID:           d.GetVendorId(),
			ProductID:          d.GetProductId(),
			Serial:             d.GetSerial(),
			Path:               d.GetPath(),
			CardID:             d.GetAlsaCardId(),
			CardName:           d.GetCardName(),
			PCMRoute:           d.GetPcmRoute(),
			SupportsVolume:     d.GetSupportsVolume(),
			SupportsMute:       d.GetSupportsMute(),
			SupportsGain:       d.GetSupportsGain(),
			SupportsAGCDisable: d.GetSupportsAgcDisable(),
			VolumeMinDB:        d.VolumeMinDb,
			VolumeMaxDB:        d.VolumeMaxDb,
			VolumeStepDB:       d.VolumeStepDb,
			GainMinDB:          d.GainMinDb,
			GainMaxDB:          d.GainMaxDb,
			GainStepDB:         d.GainStepDb,
		})
	}
	if o := r.GetOutput(); o != nil {
		out.Output = &AudioOutputObserved{
			DeviceUID:     o.GetDeviceUid(),
			VolumePercent: o.VolumePercent,
			Muted:         o.Muted,
			VolumeState:   AudioApplyState(o.GetVolumeState()),
			MuteState:     AudioApplyState(o.GetMuteState()),
		}
	}
	if i := r.GetInput(); i != nil {
		out.Input = &AudioInputObserved{
			DeviceUID:   i.GetDeviceUid(),
			GainPercent: i.GainPercent,
			GainState:   AudioApplyState(i.GetGainState()),
		}
	}
	return out
}

func helloCapabilitiesFromWire(data []byte) ([]AudioCapability, error) {
	var cm controlv2.ControlMessage
	if err := proto.Unmarshal(data, &cm); err != nil {
		return nil, fmt.Errorf("abc: decode hello: %w", err)
	}
	hello := cm.GetHello()
	if hello == nil {
		return nil, &ProtocolError{Reason: "not a hello message"}
	}
	out := make([]AudioCapability, 0, len(hello.GetCapabilities()))
	for _, cap := range hello.GetCapabilities() {
		if cap == nil {
			continue
		}
		out = append(out, AudioCapability{
			Codec:      cap.GetCodec(),
			Channels:   cap.GetChannels(),
			SampleRate: cap.GetSampleRate(),
		})
	}
	return out, nil
}
