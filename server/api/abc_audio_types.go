package api

import (
	"time"
)

// --- ABC audio settings ---

// deviceUIDPattern is a conservative pattern for canonical USB audio UIDs.
// Examples: usb:0d8c:0014:path:platform-xhci-hcd-usb-0_1_2_1_0
//
//	usb:0d8c:0014:serial:ABC123
const deviceUIDPattern = `^usb:[0-9a-fA-F]{4}:[0-9a-fA-F]{4}:(path|serial):[A-Za-z0-9._:-]{1,96}$`

// requestIDPattern allows ULID/UUID-ish opaque ids without whitespace or path chars.
const requestIDPattern = `^[A-Za-z0-9._:-]{1,64}$`

// ABCAudioOutputDesiredIn is the desired output block on PUT.
type ABCAudioOutputDesiredIn struct {
	DeviceUID     string `json:"device_uid" minLength:"1" maxLength:"128" pattern:"^usb:[0-9a-fA-F]{4}:[0-9a-fA-F]{4}:(path|serial):[A-Za-z0-9._:-]{1,96}$" doc:"Canonical USB output device UID"`
	VolumePercent int    `json:"volume_percent" minimum:"0" maximum:"100" doc:"Desired output volume percent (0-100)"`
	Muted         bool   `json:"muted" doc:"Desired output mute state"`
}

// ABCAudioInputDesiredIn is the desired input block on PUT.
type ABCAudioInputDesiredIn struct {
	DeviceUID   string `json:"device_uid" minLength:"1" maxLength:"128" pattern:"^usb:[0-9a-fA-F]{4}:[0-9a-fA-F]{4}:(path|serial):[A-Za-z0-9._:-]{1,96}$" doc:"Canonical USB input device UID"`
	GainPercent int    `json:"gain_percent" minimum:"0" maximum:"100" doc:"Desired input gain percent (0-100)"`
}

// ABCAudioDesiredOut is the durable desired snapshot in responses.
type ABCAudioDesiredOut struct {
	Revision            uint64     `json:"revision" doc:"Desired revision (0 = unconfigured)"`
	CommandID           string     `json:"command_id,omitempty" doc:"Deterministic command id for this revision"`
	OutputDeviceUID     string     `json:"output_device_uid,omitempty" doc:"Desired output device UID"`
	OutputVolumePercent *int       `json:"output_volume_percent,omitempty" doc:"Desired output volume percent"`
	OutputMuted         *bool      `json:"output_muted,omitempty" doc:"Desired output mute"`
	InputDeviceUID      string     `json:"input_device_uid,omitempty" doc:"Desired input device UID"`
	InputGainPercent    *int       `json:"input_gain_percent,omitempty" doc:"Desired input gain percent"`
	UpdatedAt           *time.Time `json:"updated_at,omitempty" doc:"When desired state last changed"`
}

// ABCAudioCapabilityOut is one discovered board audio endpoint.
type ABCAudioCapabilityOut struct {
	DeviceUID      string         `json:"device_uid" doc:"Canonical device UID"`
	Direction      string         `json:"direction,omitempty" doc:"input, output, or both"`
	Backend        string         `json:"backend,omitempty" doc:"Audio backend (e.g. alsa)"`
	VendorID       string         `json:"vendor_id,omitempty"`
	ProductID      string         `json:"product_id,omitempty"`
	Serial         string         `json:"serial,omitempty"`
	Path           string         `json:"path,omitempty"`
	ALSACardID     string         `json:"alsa_card_id,omitempty"`
	CardName       string         `json:"card_name,omitempty"`
	SupportsVolume bool           `json:"supports_volume,omitempty"`
	SupportsMute   bool           `json:"supports_mute,omitempty"`
	SupportsGain   bool           `json:"supports_gain,omitempty"`
	Extra          map[string]any `json:"extra,omitempty"`
}

// ABCAudioReportedOut is the durable observed snapshot in responses.
type ABCAudioReportedOut struct {
	Revision                    uint64                   `json:"revision" doc:"Highest conclusive reported desired revision"`
	CommandID                   string                   `json:"command_id,omitempty" doc:"Last reported command id"`
	OutputDeviceUID             string                   `json:"output_device_uid,omitempty"`
	ObservedOutputVolumePercent *int                     `json:"observed_output_volume_percent,omitempty"`
	ObservedOutputMuted         *bool                    `json:"observed_output_muted,omitempty"`
	InputDeviceUID              string                   `json:"input_device_uid,omitempty"`
	ObservedInputGainPercent    *int                     `json:"observed_input_gain_percent,omitempty"`
	OutputVolumeState           string                   `json:"output_volume_state" enum:"unknown,pending,applied,unsupported,error,device_mismatch" doc:"Per-control state"`
	OutputMuteState             string                   `json:"output_mute_state" enum:"unknown,pending,applied,unsupported,error,device_mismatch" doc:"Per-control state"`
	InputGainState              string                   `json:"input_gain_state" enum:"unknown,pending,applied,unsupported,error,device_mismatch" doc:"Per-control state"`
	ErrorCode                   string                   `json:"error_code,omitempty"`
	ErrorDetail                 string                   `json:"error_detail,omitempty"`
	Capabilities                []ABCAudioCapabilityOut  `json:"capabilities,omitempty"`
	ReportedAt                  *time.Time               `json:"reported_at,omitempty" doc:"Server receipt time of last report"`
}

// ABCAudioSettingsOut is the GET/PUT response body for ABC audio settings.
type ABCAudioSettingsOut struct {
	ABCID            string              `json:"abc_id" doc:"ABC id"`
	Connected        bool                `json:"connected" doc:"Whether the ABC is currently connected"`
	Desired          ABCAudioDesiredOut  `json:"desired"`
	Reported         ABCAudioReportedOut `json:"reported"`
	OverallState     string              `json:"overall_state" enum:"unconfigured,offline,stale,pending,error,device_mismatch,unsupported,partial,applied" doc:"Derived aggregate state"`
	Stale            bool                `json:"stale" doc:"Whether reported state is considered stale"`
	AcceptedRevision uint64              `json:"accepted_revision" doc:"Revision accepted by this request (GET echoes current desired)"`
}

// GetABCAudioSettingsRequest is GET /api/abcs/{id}/audio-settings.
type GetABCAudioSettingsRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"ABC ID"`
}

// GetABCAudioSettingsResponse is the GET response.
type GetABCAudioSettingsResponse struct {
	Body ABCAudioSettingsOut
}

// PutABCAudioSettingsRequest is PUT /api/abcs/{id}/audio-settings.
// Actor/role/ABC id/binary/mixer/command/argv are intentionally absent — audit
// actor comes only from JWT claims.
type PutABCAudioSettingsRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"ABC ID"`
	Body          struct {
		RequestID         string                   `json:"request_id" minLength:"1" maxLength:"64" pattern:"^[A-Za-z0-9._:-]{1,64}$" doc:"Client idempotency key (UUID/ULID)"`
		ExpectedRevision  uint64                   `json:"expected_revision" doc:"Expected current desired revision (0 when unconfigured)"`
		Output            ABCAudioOutputDesiredIn  `json:"output" doc:"Absolute desired output state"`
		Input             ABCAudioInputDesiredIn   `json:"input" doc:"Absolute desired input state"`
	}
}

// PutABCAudioSettingsResponse is the PUT response. Status is 200 for
// duplicate/no-op and 202 when a new revision is queued.
type PutABCAudioSettingsResponse struct {
	Status int `json:"-"`
	Body   ABCAudioSettingsOut
}
