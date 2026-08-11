package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// handleGetABCAudioSettings returns durable desired + reported audio status.
func (s *Server) handleGetABCAudioSettings(ctx context.Context, input *GetABCAudioSettingsRequest) (*GetABCAudioSettingsResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}
	if s.services.ABCAudio == nil {
		return nil, huma.Error500InternalServerError("abc audio service not configured")
	}

	st, err := s.services.ABCAudio.Get(ctx, input.ID)
	if err != nil {
		return nil, mapABCAudioError(err)
	}
	// Ensure overall state is derived with connected fact from store.
	st.OverallState = crosstalk.DeriveABCAudioOverallState(*st)

	resp := &GetABCAudioSettingsResponse{}
	resp.Body = abcAudioStatusToOut(st)
	// GET echoes current desired revision as accepted_revision for client convenience.
	if resp.Body.AcceptedRevision == 0 {
		resp.Body.AcceptedRevision = st.Desired.Revision
	}
	return resp, nil
}

// handlePutABCAudioSettings absolutely replaces desired audio state.
// Returns 202 when a new revision is queued, 200 for duplicate request_id or no-op.
func (s *Server) handlePutABCAudioSettings(ctx context.Context, input *PutABCAudioSettingsRequest) (*PutABCAudioSettingsResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}
	if s.services.ABCAudio == nil {
		return nil, huma.Error500InternalServerError("abc audio service not configured")
	}

	desired := crosstalk.ABCAudioDesired{
		OutputDeviceUID:     input.Body.Output.DeviceUID,
		OutputVolumePercent: input.Body.Output.VolumePercent,
		OutputMuted:         input.Body.Output.Muted,
		InputDeviceUID:      input.Body.Input.DeviceUID,
		InputGainPercent:    input.Body.Input.GainPercent,
	}
	if err := crosstalk.ValidateABCAudioDesired(desired); err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}

	// Capability / rebind gate (service persists without this rule).
	if err := s.enforceABCAudioDeviceBinding(ctx, input.ID, desired); err != nil {
		return nil, err
	}

	// Snapshot revision before SetDesired so we can distinguish 202 vs 200.
	pre, err := s.services.ABCAudio.Get(ctx, input.ID)
	if err != nil {
		return nil, mapABCAudioError(err)
	}
	preRev := pre.Desired.Revision

	// Actor ONLY from JWT claims — never from body.
	st, err := s.services.ABCAudio.SetDesired(
		ctx,
		input.ID,
		claims.Subject,
		claims.Role,
		input.Body.RequestID,
		input.Body.ExpectedRevision,
		desired,
	)
	if err != nil {
		return nil, mapABCAudioError(err)
	}
	st.OverallState = crosstalk.DeriveABCAudioOverallState(*st)

	resp := &PutABCAudioSettingsResponse{}
	resp.Body = abcAudioStatusToOut(st)
	if st.Desired.Revision > preRev {
		resp.Status = http.StatusAccepted // 202 new revision queued
	} else {
		resp.Status = http.StatusOK // 200 duplicate / no-op
	}

	// Lane D will hook reconcile/push after durable accept. Persist-only here.
	// TODO(lane-d): best-effort reconcile/send AudioControlCommand after SetDesired.

	return resp, nil
}

// enforceABCAudioDeviceBinding applies the plan's capability rules:
//   - Initial save (desired rev 0): both UIDs must appear in reported capabilities.
//   - Later offline edits: may target already-bound desired UIDs without fresh caps.
//   - Rebind to a new UID: requires current capability evidence for that UID.
func (s *Server) enforceABCAudioDeviceBinding(ctx context.Context, abcID string, desired crosstalk.ABCAudioDesired) error {
	st, err := s.services.ABCAudio.Get(ctx, abcID)
	if err != nil {
		return mapABCAudioError(err)
	}

	capUIDs := capabilityUIDSet(st.Reported.Capabilities)
	boundOut := st.Desired.OutputDeviceUID
	boundIn := st.Desired.InputDeviceUID
	configured := st.Desired.Revision > 0

	if !configured {
		// Initial save requires reported capability containing both UIDs.
		if !capUIDs[desired.OutputDeviceUID] || !capUIDs[desired.InputDeviceUID] {
			return huma.Error409Conflict("initial audio save requires reported capability for both device UIDs")
		}
		return nil
	}

	// Already-bound UIDs are always allowed (offline same-device update).
	outOK := desired.OutputDeviceUID == boundOut || capUIDs[desired.OutputDeviceUID]
	inOK := desired.InputDeviceUID == boundIn || capUIDs[desired.InputDeviceUID]
	if !outOK || !inOK {
		return huma.Error409Conflict("device rebind requires current capability evidence for the new UID")
	}
	return nil
}

func capabilityUIDSet(caps []crosstalk.ABCAudioCapability) map[string]bool {
	out := make(map[string]bool, len(caps))
	for _, c := range caps {
		if c.DeviceUID != "" {
			out[c.DeviceUID] = true
		}
	}
	return out
}

func mapABCAudioError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, crosstalk.ErrABCAudioABCNotFound):
		return huma.Error404NotFound("ABC not found")
	case errors.Is(err, crosstalk.ErrABCAudioNotFound):
		return huma.Error404NotFound("ABC audio settings not found")
	case errors.Is(err, crosstalk.ErrABCAudioRevisionConflict):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, crosstalk.ErrABCAudioInvalidDesired):
		return huma.Error422UnprocessableEntity(err.Error())
	case errors.Is(err, crosstalk.ErrABCAudioInvalidReport):
		return huma.Error422UnprocessableEntity(err.Error())
	default:
		return huma.Error500InternalServerError(fmt.Sprintf("abc audio error: %v", err))
	}
}

func abcAudioStatusToOut(st *crosstalk.ABCAudioStatus) ABCAudioSettingsOut {
	if st == nil {
		return ABCAudioSettingsOut{}
	}
	return ABCAudioSettingsOut{
		ABCID:            st.ABCID,
		Connected:        st.Connected,
		Desired:          abcAudioDesiredToOut(st.Desired),
		Reported:         abcAudioReportedToOut(st.Reported),
		OverallState:     string(st.OverallState),
		Stale:            st.Stale,
		AcceptedRevision: st.AcceptedRevision,
	}
}

func abcAudioDesiredToOut(d crosstalk.ABCAudioDesiredView) ABCAudioDesiredOut {
	return ABCAudioDesiredOut{
		Revision:            d.Revision,
		CommandID:           d.CommandID,
		OutputDeviceUID:     d.OutputDeviceUID,
		OutputVolumePercent: d.OutputVolumePercent,
		OutputMuted:         d.OutputMuted,
		InputDeviceUID:      d.InputDeviceUID,
		InputGainPercent:    d.InputGainPercent,
		UpdatedAt:           d.UpdatedAt,
	}
}

func abcAudioReportedToOut(r crosstalk.ABCAudioReportedView) ABCAudioReportedOut {
	caps := make([]ABCAudioCapabilityOut, 0, len(r.Capabilities))
	for _, c := range r.Capabilities {
		caps = append(caps, ABCAudioCapabilityOut{
			DeviceUID:      c.DeviceUID,
			Direction:      c.Direction,
			Backend:        c.Backend,
			VendorID:       c.VendorID,
			ProductID:      c.ProductID,
			Serial:         c.Serial,
			Path:           c.Path,
			ALSACardID:     c.ALSACardID,
			CardName:       c.CardName,
			SupportsVolume: c.SupportsVolume,
			SupportsMute:   c.SupportsMute,
			SupportsGain:   c.SupportsGain,
			Extra:          c.Extra,
		})
	}
	volState := string(r.OutputVolumeState)
	if volState == "" {
		volState = string(crosstalk.ABCAudioControlUnknown)
	}
	muteState := string(r.OutputMuteState)
	if muteState == "" {
		muteState = string(crosstalk.ABCAudioControlUnknown)
	}
	gainState := string(r.InputGainState)
	if gainState == "" {
		gainState = string(crosstalk.ABCAudioControlUnknown)
	}
	return ABCAudioReportedOut{
		Revision:                    r.Revision,
		CommandID:                   r.CommandID,
		OutputDeviceUID:             r.OutputDeviceUID,
		ObservedOutputVolumePercent: r.ObservedOutputVolumePercent,
		ObservedOutputMuted:         r.ObservedOutputMuted,
		InputDeviceUID:              r.InputDeviceUID,
		ObservedInputGainPercent:    r.ObservedInputGainPercent,
		OutputVolumeState:           volState,
		OutputMuteState:             muteState,
		InputGainState:              gainState,
		ErrorCode:                   r.ErrorCode,
		ErrorDetail:                 r.ErrorDetail,
		Capabilities:                caps,
		ReportedAt:                  r.ReportedAt,
	}
}
