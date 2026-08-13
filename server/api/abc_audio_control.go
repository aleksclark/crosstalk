package api

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"

	crosstalk "github.com/aleksclark/crosstalk/server"
	crosstalkv2 "github.com/aleksclark/crosstalk/server/proto/v2"
	"github.com/aleksclark/crosstalk/server/webrtc"
)

// abcPeerEntry is the process-local live ABC control-channel peer.
// Generation is monotonically increasing per ABC so a stale close cannot
// remove a newer connection.
type abcPeerEntry struct {
	PeerID     string
	Peer       *webrtc.PeerConn
	Generation uint64
}

// MaxABCAudioDevices is the maximum devices list accepted from a board report.
const MaxABCAudioDevices = 8

// MaxABCAudioStringBytes caps individual string fields from board reports.
const MaxABCAudioStringBytes = 128

// abcAudioDBTimeout bounds DB work kicked from data-channel callbacks.
const abcAudioDBTimeout = 5 * time.Second

// registerABCPeer records the live signaling peer for an ABC and returns the
// connection generation assigned to this registration.
func (s *Server) registerABCPeer(abcID string, peer *webrtc.PeerConn) uint64 {
	if peer == nil {
		return 0
	}
	s.abcPeersMu.Lock()
	defer s.abcPeersMu.Unlock()
	s.abcPeerGen++
	gen := s.abcPeerGen
	s.abcPeers[abcID] = abcPeerEntry{
		PeerID:     peer.ID,
		Peer:       peer,
		Generation: gen,
	}
	return gen
}

// deregisterABCPeer clears the live peer mapping for an ABC, but only if it
// still points at the same peer ID and generation (a newer connection may have
// already replaced it).
func (s *Server) deregisterABCPeer(abcID, peerID string, generation uint64) {
	s.abcPeersMu.Lock()
	defer s.abcPeersMu.Unlock()
	cur, ok := s.abcPeers[abcID]
	if !ok {
		return
	}
	if cur.PeerID == peerID && cur.Generation == generation {
		delete(s.abcPeers, abcID)
	}
}

// lookupABCPeer returns a snapshot of the current live peer entry.
func (s *Server) lookupABCPeer(abcID string) (abcPeerEntry, bool) {
	s.abcPeersMu.Lock()
	defer s.abcPeersMu.Unlock()
	e, ok := s.abcPeers[abcID]
	return e, ok
}

// reconnectABC closes an ABC's live signaling peer, if any, so the
// auto-reconnecting board re-establishes its connection and re-bridges with its
// current session/monitor assignment. No-op when the ABC is not connected.
func (s *Server) reconnectABC(abcID string) {
	e, ok := s.lookupABCPeer(abcID)
	if !ok || e.PeerID == "" || s.services.PeerManager == nil {
		return
	}
	peerID := e.PeerID
	s.log.Info("reconnecting abc after assignment change", "abc", abcID, "peer", peerID, "generation", e.Generation)
	// Async: PeerConn.Close can block on Pion teardown under load.
	go s.services.PeerManager.RemovePeer(peerID)
}

// restartABC closes the current live peer so the board's reconnect loop creates
// a fresh control/media connection. It reports false when no live peer exists.
// Peer Close can block on Pion teardown, so removal runs asynchronously after
// the live-peer check — the HTTP restart handler must not hang.
func (s *Server) restartABC(abcID string) bool {
	e, ok := s.lookupABCPeer(abcID)
	if !ok || e.PeerID == "" || s.services.PeerManager == nil {
		return false
	}
	if s.services.PeerManager.FindPeer(e.PeerID) == nil {
		return false
	}
	peerID := e.PeerID
	gen := e.Generation
	s.log.Info("restarting abc connection", "abc", abcID, "peer", peerID, "generation", gen)
	go s.services.PeerManager.RemovePeer(peerID)
	return true
}

// handleABCAudioControlReport persists a board report bound to the authenticated
// ABC ID, then reconciles durable desired state onto the live peer.
// The report cannot nominate another ABC — abcID comes only from admission.
func (s *Server) handleABCAudioControlReport(abcID string, peer *webrtc.PeerConn, report *crosstalkv2.AudioControlReport) {
	if abcID == "" || report == nil || s.services.ABCAudio == nil {
		return
	}
	obs, ok := mapAudioControlReport(report)
	if !ok {
		s.log.Warn("abc audio report rejected",
			"abc", abcID,
			"peer", peerID(peer),
			"reason", "malformed_or_limits",
		)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), abcAudioDBTimeout)
	defer cancel()

	st, err := s.services.ABCAudio.RecordReport(ctx, abcID, obs)
	if err != nil {
		s.log.Warn("abc audio report persist failed",
			"abc", abcID,
			"peer", peerID(peer),
			"desired_revision", obs.DesiredRevision,
			"error_code", safeErrCode(err),
		)
		return
	}
	s.log.Info("abc audio report recorded",
		"abc", abcID,
		"peer", peerID(peer),
		"report_revision", st.Reported.Revision,
		"desired_revision", st.Desired.Revision,
		"command_id", st.Reported.CommandID,
		"overall", string(crosstalk.DeriveABCAudioOverallState(*st)),
	)
	s.reconcileABCAudio(abcID)
}

// reconcileABCAudio reads durable status and, when the board is behind or
// non-conclusive, best-effort sends AudioControlCommand on the live control
// channel. Send failure leaves desired pending and does not roll back.
func (s *Server) reconcileABCAudio(abcID string) {
	s.reconcileABCAudioOpts(abcID, false)
}

// reconcileABCAudioForce always re-pushes durable desired when configured.
// Used on control-open/Hello so reboot/restart re-applies absolute mixer state
// even if the last durable report was already "applied" (hardware may have
// been reset by setup helpers or driver init).
func (s *Server) reconcileABCAudioForce(abcID string) {
	s.reconcileABCAudioOpts(abcID, true)
}

func (s *Server) reconcileABCAudioOpts(abcID string, force bool) {
	if abcID == "" || s.services.ABCAudio == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), abcAudioDBTimeout)
	defer cancel()

	st, err := s.services.ABCAudio.Get(ctx, abcID)
	if err != nil {
		s.log.Warn("abc audio reconcile get failed",
			"abc", abcID,
			"error_code", safeErrCode(err),
		)
		return
	}
	if st.Desired.Revision == 0 {
		// Unconfigured: inventory only; never send a command.
		return
	}
	if !force && !abcAudioNeedsCommand(st) {
		return
	}

	e, ok := s.lookupABCPeer(abcID)
	if !ok || e.Peer == nil {
		s.log.Debug("abc audio reconcile: no live peer",
			"abc", abcID,
			"desired_revision", st.Desired.Revision,
			"command_id", st.Desired.CommandID,
			"force", force,
		)
		return
	}

	cmd := buildAudioControlCommand(st)
	if cmd == nil {
		return
	}
	msg := &crosstalkv2.ControlMessage{
		Payload: &crosstalkv2.ControlMessage_AudioControlCommand{
			AudioControlCommand: cmd,
		},
	}
	if err := e.Peer.SendControlMessage(msg); err != nil {
		s.log.Warn("abc audio command send failed",
			"abc", abcID,
			"peer", e.PeerID,
			"generation", e.Generation,
			"desired_revision", st.Desired.Revision,
			"command_id", st.Desired.CommandID,
			"report_revision", st.Reported.Revision,
			"force", force,
			"error_code", safeErrCode(err),
		)
		return
	}
	s.log.Info("abc audio command sent",
		"abc", abcID,
		"peer", e.PeerID,
		"generation", e.Generation,
		"desired_revision", st.Desired.Revision,
		"command_id", st.Desired.CommandID,
		"report_revision", st.Reported.Revision,
		"force", force,
		"state", "pending",
	)
}

// abcAudioNeedsCommand reports whether durable desired should be pushed.
func abcAudioNeedsCommand(st *crosstalk.ABCAudioStatus) bool {
	if st == nil || st.Desired.Revision == 0 {
		return false
	}
	if st.Reported.Revision < st.Desired.Revision {
		return true
	}
	// Same or newer report revision: resend only when non-conclusive or the
	// desired device has returned after a device_mismatch.
	vol := st.Reported.OutputVolumeState
	mute := st.Reported.OutputMuteState
	gain := st.Reported.InputGainState
	if vol == "" {
		vol = crosstalk.ABCAudioControlUnknown
	}
	if mute == "" {
		mute = crosstalk.ABCAudioControlUnknown
	}
	if gain == "" {
		gain = crosstalk.ABCAudioControlUnknown
	}
	if vol == crosstalk.ABCAudioControlUnknown || vol == crosstalk.ABCAudioControlPending ||
		mute == crosstalk.ABCAudioControlUnknown || mute == crosstalk.ABCAudioControlPending ||
		gain == crosstalk.ABCAudioControlUnknown || gain == crosstalk.ABCAudioControlPending {
		return true
	}
	if vol == crosstalk.ABCAudioControlDeviceMismatch ||
		mute == crosstalk.ABCAudioControlDeviceMismatch ||
		gain == crosstalk.ABCAudioControlDeviceMismatch {
		// Only re-push when the desired UIDs are present again in capabilities.
		return desiredDevicesPresent(st)
	}
	return false
}

func desiredDevicesPresent(st *crosstalk.ABCAudioStatus) bool {
	if st == nil {
		return false
	}
	have := capabilityUIDSet(st.Reported.Capabilities)
	outOK := st.Desired.OutputDeviceUID == "" || have[st.Desired.OutputDeviceUID]
	inOK := st.Desired.InputDeviceUID == "" || have[st.Desired.InputDeviceUID]
	return outOK && inOK
}

// buildAudioControlCommand builds the deterministic absolute command from status.
func buildAudioControlCommand(st *crosstalk.ABCAudioStatus) *crosstalkv2.AudioControlCommand {
	if st == nil || st.Desired.Revision == 0 {
		return nil
	}
	cmdID := st.Desired.CommandID
	if cmdID == "" {
		cmdID = crosstalk.ABCAudioCommandID(st.ABCID, st.Desired.Revision)
	}
	out := &crosstalkv2.AudioOutputDesired{
		DeviceUid: st.Desired.OutputDeviceUID,
	}
	if st.Desired.OutputVolumePercent != nil {
		v := uint32(*st.Desired.OutputVolumePercent)
		out.VolumePercent = &v
	}
	if st.Desired.OutputMuted != nil {
		m := *st.Desired.OutputMuted
		out.Muted = &m
	}
	in := &crosstalkv2.AudioInputDesired{
		DeviceUid: st.Desired.InputDeviceUID,
	}
	if st.Desired.InputGainPercent != nil {
		g := uint32(*st.Desired.InputGainPercent)
		in.GainPercent = &g
	}
	return &crosstalkv2.AudioControlCommand{
		CommandId:       cmdID,
		DesiredRevision: st.Desired.Revision,
		Output:          out,
		Input:           in,
	}
}

// mapAudioControlReport converts a protobuf report into a domain observation.
// Returns ok=false when limits/enums are violated (caller must not persist).
func mapAudioControlReport(report *crosstalkv2.AudioControlReport) (crosstalk.ABCAudioObservation, bool) {
	var zero crosstalk.ABCAudioObservation
	if report == nil {
		return zero, false
	}
	if len(report.GetDevices()) > MaxABCAudioDevices {
		return zero, false
	}
	if !boundedString(report.GetCommandId(), MaxABCAudioStringBytes) ||
		!boundedString(report.GetErrorCode(), MaxABCAudioStringBytes) {
		return zero, false
	}
	// error_detail is sanitized/capped by the store; still bound wire size.
	if len(report.GetErrorDetail()) > crosstalk.MaxABCAudioErrorDetailBytes*2 {
		return zero, false
	}

	obs := crosstalk.ABCAudioObservation{
		DesiredRevision: report.GetDesiredRevision(),
		CommandID:       report.GetCommandId(),
		ErrorCode:       report.GetErrorCode(),
		ErrorDetail:     report.GetErrorDetail(),
		ReportedAt:      time.Now().UTC(),
	}

	if out := report.GetOutput(); out != nil {
		if !boundedString(out.GetDeviceUid(), MaxABCAudioStringBytes) {
			return zero, false
		}
		obs.OutputDeviceUID = out.GetDeviceUid()
		if out.VolumePercent != nil {
			v := int(*out.VolumePercent)
			if v < 0 || v > 100 {
				return zero, false
			}
			obs.ObservedOutputVolumePercent = &v
		}
		if out.Muted != nil {
			m := *out.Muted
			obs.ObservedOutputMuted = &m
		}
		volState, ok := mapAudioApplyState(out.GetVolumeState())
		if !ok {
			return zero, false
		}
		muteState, ok := mapAudioApplyState(out.GetMuteState())
		if !ok {
			return zero, false
		}
		obs.OutputVolumeState = volState
		obs.OutputMuteState = muteState
	} else {
		obs.OutputVolumeState = crosstalk.ABCAudioControlUnknown
		obs.OutputMuteState = crosstalk.ABCAudioControlUnknown
	}

	if in := report.GetInput(); in != nil {
		if !boundedString(in.GetDeviceUid(), MaxABCAudioStringBytes) {
			return zero, false
		}
		obs.InputDeviceUID = in.GetDeviceUid()
		if in.GainPercent != nil {
			g := int(*in.GainPercent)
			if g < 0 || g > 100 {
				return zero, false
			}
			obs.ObservedInputGainPercent = &g
		}
		gainState, ok := mapAudioApplyState(in.GetGainState())
		if !ok {
			return zero, false
		}
		obs.InputGainState = gainState
	} else {
		obs.InputGainState = crosstalk.ABCAudioControlUnknown
	}

	// Inventory-only (rev 0) may omit conclusive control states.
	if obs.DesiredRevision == 0 {
		if obs.OutputVolumeState == "" {
			obs.OutputVolumeState = crosstalk.ABCAudioControlUnknown
		}
		if obs.OutputMuteState == "" {
			obs.OutputMuteState = crosstalk.ABCAudioControlUnknown
		}
		if obs.InputGainState == "" {
			obs.InputGainState = crosstalk.ABCAudioControlUnknown
		}
	}

	caps := make([]crosstalk.ABCAudioCapability, 0, len(report.GetDevices()))
	for _, d := range report.GetDevices() {
		if d == nil {
			continue
		}
		if !boundedString(d.GetDeviceUid(), MaxABCAudioStringBytes) ||
			!boundedString(d.GetDirection(), MaxABCAudioStringBytes) ||
			!boundedString(d.GetBackend(), MaxABCAudioStringBytes) ||
			!boundedString(d.GetVendorId(), MaxABCAudioStringBytes) ||
			!boundedString(d.GetProductId(), MaxABCAudioStringBytes) ||
			!boundedString(d.GetSerial(), MaxABCAudioStringBytes) ||
			!boundedString(d.GetPath(), MaxABCAudioStringBytes) ||
			!boundedString(d.GetAlsaCardId(), MaxABCAudioStringBytes) ||
			!boundedString(d.GetCardName(), MaxABCAudioStringBytes) {
			return zero, false
		}
		cap := crosstalk.ABCAudioCapability{
			DeviceUID:      d.GetDeviceUid(),
			Direction:      d.GetDirection(),
			Backend:        d.GetBackend(),
			VendorID:       d.GetVendorId(),
			ProductID:      d.GetProductId(),
			Serial:         d.GetSerial(),
			Path:           d.GetPath(),
			ALSACardID:     d.GetAlsaCardId(),
			CardName:       d.GetCardName(),
			SupportsVolume: d.GetSupportsVolume(),
			SupportsMute:   d.GetSupportsMute(),
			SupportsGain:   d.GetSupportsGain(),
		}
		if pcm := d.GetPcmRoute(); pcm != "" {
			if !boundedString(pcm, MaxABCAudioStringBytes) {
				return zero, false
			}
			cap.Extra = map[string]string{"pcm_route": pcm}
		}
		caps = append(caps, cap)
	}
	obs.Capabilities = caps
	return obs, true
}

// mapAudioApplyState maps proto enum → domain control state.
// STALE_REVISION is handled as unknown so reconcile can re-push if needed;
// unspecified maps to unknown. Unknown enum values are rejected.
func mapAudioApplyState(st crosstalkv2.AudioApplyState) (crosstalk.ABCAudioControlState, bool) {
	switch st {
	case crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_UNSPECIFIED:
		return crosstalk.ABCAudioControlUnknown, true
	case crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_APPLIED:
		return crosstalk.ABCAudioControlApplied, true
	case crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_UNSUPPORTED:
		return crosstalk.ABCAudioControlUnsupported, true
	case crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_ERROR:
		return crosstalk.ABCAudioControlError, true
	case crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_DEVICE_MISMATCH:
		return crosstalk.ABCAudioControlDeviceMismatch, true
	case crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_STALE_REVISION:
		// Board rejected an older command; treat as non-conclusive so a newer
		// desired revision still reconciles. Does not invent "applied".
		return crosstalk.ABCAudioControlUnknown, true
	default:
		return "", false
	}
}

func boundedString(s string, max int) bool {
	return len(s) <= max
}

func peerID(peer *webrtc.PeerConn) string {
	if peer == nil {
		return ""
	}
	return peer.ID
}

func safeErrCode(err error) string {
	if err == nil {
		return ""
	}
	// Never log full error strings that might contain board stderr/config.
	switch err {
	case context.DeadlineExceeded:
		return "deadline_exceeded"
	case context.Canceled:
		return "canceled"
	default:
		// Use error type / sentinel string only when short.
		msg := err.Error()
		if len(msg) > 64 {
			return "error"
		}
		return msg
	}
}

// Ensure proto import used when tests build command bytes.
var _ = proto.Marshal
