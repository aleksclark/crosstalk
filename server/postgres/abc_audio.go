package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/uptrace/bun"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// ABCAudioStore implements crosstalk.ABCAudioService.
type ABCAudioStore struct {
	db *DB
}

// NewABCAudioStore constructs an ABC audio settings store.
func NewABCAudioStore(db *DB) *ABCAudioStore {
	return &ABCAudioStore{db: db}
}

// Compile-time check.
var _ crosstalk.ABCAudioService = (*ABCAudioStore)(nil)

// Get returns durable audio status for an existing ABC. Missing settings rows
// yield an unconfigured status without creating a row.
func (s *ABCAudioStore) Get(ctx context.Context, abcID string) (*crosstalk.ABCAudioStatus, error) {
	abc, err := s.loadABC(ctx, s.db, abcID)
	if err != nil {
		return nil, err
	}
	m, err := s.selectSettings(ctx, s.db, abcID, false)
	if errors.Is(err, sql.ErrNoRows) {
		st := crosstalk.UnconfiguredABCAudioStatus(abcID, abc.Connected)
		st.OverallState = crosstalk.DeriveABCAudioOverallState(*st)
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	return s.statusFromModel(m, abc.Connected), nil
}

// SetDesired absolutely replaces desired state under revision/idempotency rules.
func (s *ABCAudioStore) SetDesired(ctx context.Context, abcID, actorID, actorRole, requestID string,
	expectedRevision uint64, desired crosstalk.ABCAudioDesired) (*crosstalk.ABCAudioStatus, error) {

	if err := validateSetDesiredInput(actorID, actorRole, requestID, desired); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	abc, err := s.loadABC(ctx, tx, abcID)
	if err != nil {
		return nil, err
	}

	// Fast path: prior (abc_id, request_id) without waiting on the settings lock.
	if prior, err := s.findAuditByRequest(ctx, tx, abcID, requestID); err == nil {
		m, err := s.selectSettings(ctx, tx, abcID, false)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		st := s.statusFromModel(m, abc.Connected)
		st.AcceptedRevision = uint64(prior.DesiredRevision)
		return st, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// Lock settings row if present; create empty unconfigured row if missing.
	// Under concurrency the row lock serializes desired/revision updates.
	m, err := s.selectSettings(ctx, tx, abcID, true)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err = s.insertEmptySettings(ctx, tx, abcID); err != nil {
			return nil, err
		}
		// Re-lock the inserted row.
		m, err = s.selectSettings(ctx, tx, abcID, true)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	// Re-check idempotency after the lock so concurrent duplicate request_ids
	// return the original accepted revision instead of racing into conflict.
	if prior, err := s.findAuditByRequest(ctx, tx, abcID, requestID); err == nil {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		st := s.statusFromModel(m, abc.Connected)
		// Reload unlocked view for freshness (m was locked pre-image).
		if m2, err := s.selectSettings(ctx, s.db, abcID, false); err == nil {
			st = s.statusFromModel(m2, abc.Connected)
		}
		st.AcceptedRevision = uint64(prior.DesiredRevision)
		return st, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	currentRev := uint64(m.DesiredRevision)
	if currentRev != expectedRevision {
		return nil, fmt.Errorf("%w: expected %d have %d", crosstalk.ErrABCAudioRevisionConflict, expectedRevision, currentRev)
	}

	prevView := desiredViewFromModel(m)
	prevDesired, hasPrev := desiredFromModel(m)

	// Byte-for-byte equal desired is a successful no-op.
	if hasPrev && desiredContentEqual(prevDesired, desired) {
		if err := s.insertAudit(ctx, tx, abcID, actorID, actorRole, requestID,
			currentRev, prevView, prevView, crosstalk.ABCAudioAuditNoOp); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		st := s.statusFromModel(m, abc.Connected)
		st.AcceptedRevision = currentRev
		return st, nil
	}

	newRev := currentRev + 1
	now := time.Now().UTC()
	cmdID := crosstalk.ABCAudioCommandID(abcID, newRev)
	outUID := desired.OutputDeviceUID
	inUID := desired.InputDeviceUID
	vol := int16(desired.OutputVolumePercent)
	gain := int16(desired.InputGainPercent)
	muted := desired.OutputMuted

	// Mark controls pending when desired advances.
	m.DesiredRevision = int64(newRev)
	m.DesiredOutputDeviceUID = &outUID
	m.DesiredOutputVolumePercent = &vol
	m.DesiredOutputMuted = &muted
	m.DesiredInputDeviceUID = &inUID
	m.DesiredInputGainPercent = &gain
	m.CommandID = &cmdID
	m.DesiredUpdatedAt = &now
	m.UpdatedAt = now
	m.OutputVolumeState = string(crosstalk.ABCAudioControlPending)
	m.OutputMuteState = string(crosstalk.ABCAudioControlPending)
	m.InputGainState = string(crosstalk.ABCAudioControlPending)
	// Clear prior error on new desired.
	m.ErrorCode = ""
	m.ErrorDetail = ""

	if _, err := tx.NewUpdate().Model(m).
		Column(
			"desired_revision",
			"desired_output_device_uid",
			"desired_output_volume_percent",
			"desired_output_muted",
			"desired_input_device_uid",
			"desired_input_gain_percent",
			"command_id",
			"desired_updated_at",
			"updated_at",
			"output_volume_state",
			"output_mute_state",
			"input_gain_state",
			"error_code",
			"error_detail",
		).
		WherePK().
		Exec(ctx); err != nil {
		return nil, fmt.Errorf("update abc audio desired: %w", err)
	}

	newView := desiredViewFromModel(m)
	if err := s.insertAudit(ctx, tx, abcID, actorID, actorRole, requestID,
		newRev, prevView, newView, crosstalk.ABCAudioAuditAccepted); err != nil {
		return nil, err
	}

	// Reload for consistent status.
	m2, err := s.selectSettings(ctx, tx, abcID, false)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	st := s.statusFromModel(m2, abc.Connected)
	st.AcceptedRevision = newRev
	return st, nil
}

// RecordReport persists a board observation with stale/inventory rules.
func (s *ABCAudioStore) RecordReport(ctx context.Context, abcID string, report crosstalk.ABCAudioObservation) (*crosstalk.ABCAudioStatus, error) {
	if err := validateReport(report); err != nil {
		return nil, err
	}
	report.ErrorDetail = crosstalk.SanitizeABCAudioErrorDetail(report.ErrorDetail)
	if report.ReportedAt.IsZero() {
		report.ReportedAt = time.Now().UTC()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	abc, err := s.loadABC(ctx, tx, abcID)
	if err != nil {
		return nil, err
	}

	m, err := s.selectSettings(ctx, tx, abcID, true)
	if errors.Is(err, sql.ErrNoRows) {
		// First inventory (or any first report) may create revision-0 row.
		if _, err = s.insertEmptySettings(ctx, tx, abcID); err != nil {
			return nil, err
		}
		m, err = s.selectSettings(ctx, tx, abcID, true)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	// Inventory-only revision 0: update capabilities (and optional device UIDs /
	// error fields) but never overwrite applied desired state or roll back
	// reported revision.
	if report.DesiredRevision == 0 {
		if err := s.applyInventoryReport(ctx, tx, m, report); err != nil {
			return nil, err
		}
		m2, err := s.selectSettings(ctx, tx, abcID, false)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return s.statusFromModel(m2, abc.Connected), nil
	}

	// Ignore stale reports whose desired revision is lower than persisted reported.
	if report.DesiredRevision < uint64(m.ReportedRevision) {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return s.statusFromModel(m, abc.Connected), nil
	}

	if err := s.applyObservationReport(ctx, tx, m, report); err != nil {
		return nil, err
	}
	m2, err := s.selectSettings(ctx, tx, abcID, false)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.statusFromModel(m2, abc.Connected), nil
}

// ListAudit returns recent audit events newest-first.
func (s *ABCAudioStore) ListAudit(ctx context.Context, abcID string, limit int) ([]crosstalk.ABCAudioAuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	var models []abcAudioAuditModel
	err := s.db.NewSelect().Model(&models).
		Where("abc_id = ?", abcID).
		Order("created_at DESC").
		Limit(limit).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]crosstalk.ABCAudioAuditEvent, 0, len(models))
	for i := range models {
		ev, err := auditToDomain(&models[i])
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// internals
// ---------------------------------------------------------------------------

type bunIDB interface {
	NewSelect() *bun.SelectQuery
	NewInsert() *bun.InsertQuery
	NewUpdate() *bun.UpdateQuery
	NewRaw(query string, args ...any) *bun.RawQuery
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *ABCAudioStore) loadABC(ctx context.Context, db bunIDB, abcID string) (*crosstalk.ABC, error) {
	m := new(abcModel)
	err := db.NewSelect().Model(m).Where("id = ?", abcID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", crosstalk.ErrABCAudioABCNotFound, abcID)
	}
	if err != nil {
		return nil, err
	}
	out := m.toDomain()
	return &out, nil
}

func (s *ABCAudioStore) selectSettings(ctx context.Context, db bunIDB, abcID string, forUpdate bool) (*abcAudioSettingsModel, error) {
	m := new(abcAudioSettingsModel)
	q := db.NewSelect().Model(m).Where("abc_id = ?", abcID)
	if forUpdate {
		q = q.For("UPDATE")
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *ABCAudioStore) insertEmptySettings(ctx context.Context, db bunIDB, abcID string) (*abcAudioSettingsModel, error) {
	now := time.Now().UTC()
	m := &abcAudioSettingsModel{
		ABCID:             abcID,
		DesiredRevision:   0,
		ReportedRevision:  0,
		OutputVolumeState: string(crosstalk.ABCAudioControlUnknown),
		OutputMuteState:   string(crosstalk.ABCAudioControlUnknown),
		InputGainState:    string(crosstalk.ABCAudioControlUnknown),
		ErrorCode:         "",
		ErrorDetail:       "",
		Capabilities:      json.RawMessage(`{}`),
		UpdatedAt:         now,
	}
	// Use ON CONFLICT DO NOTHING so concurrent first-writers don't fail.
	_, err := db.NewRaw(`
		INSERT INTO abc_audio_settings (
			abc_id, desired_revision, reported_revision,
			output_volume_state, output_mute_state, input_gain_state,
			error_code, error_detail, capabilities, updated_at
		) VALUES (?, 0, 0, 'unknown', 'unknown', 'unknown', '', '', '{}'::jsonb, ?)
		ON CONFLICT (abc_id) DO NOTHING
	`, abcID, now).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("insert empty abc audio settings: %w", err)
	}
	return m, nil
}

func (s *ABCAudioStore) findAuditByRequest(ctx context.Context, db bunIDB, abcID, requestID string) (*abcAudioAuditModel, error) {
	m := new(abcAudioAuditModel)
	err := db.NewSelect().Model(m).
		Where("abc_id = ?", abcID).
		Where("request_id = ?", requestID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s *ABCAudioStore) insertAudit(ctx context.Context, db bunIDB, abcID, actorID, actorRole, requestID string,
	desiredRev uint64, prev, next crosstalk.ABCAudioDesiredView, outcome crosstalk.ABCAudioAuditOutcome) error {

	prevJSON, err := json.Marshal(desiredViewDTO(prev))
	if err != nil {
		return err
	}
	nextJSON, err := json.Marshal(desiredViewDTO(next))
	if err != nil {
		return err
	}
	m := &abcAudioAuditModel{
		ID:              ulid.Make().String(),
		ABCID:           abcID,
		RequestID:       requestID,
		ActorUserID:     actorID,
		ActorRole:       actorRole,
		DesiredRevision: int64(desiredRev),
		PreviousDesired: json.RawMessage(prevJSON),
		NewDesired:      json.RawMessage(nextJSON),
		Outcome:         string(outcome),
		CreatedAt:       time.Now().UTC(),
	}
	_, err = db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		// Unique (abc_id, request_id) race: treat as success path via caller re-check.
		return fmt.Errorf("insert abc audio audit: %w", err)
	}
	return nil
}

func (s *ABCAudioStore) applyInventoryReport(ctx context.Context, db bunIDB, m *abcAudioSettingsModel, report crosstalk.ABCAudioObservation) error {
	capsJSON, err := marshalCapabilities(report.Capabilities)
	if err != nil {
		return err
	}
	now := report.ReportedAt.UTC()
	m.Capabilities = capsJSON
	m.ReportedAt = &now
	m.UpdatedAt = time.Now().UTC()
	// Optional device UIDs / errors on inventory without changing reported_revision.
	if report.OutputDeviceUID != "" {
		uid := report.OutputDeviceUID
		m.ReportedOutputDeviceUID = &uid
	}
	if report.InputDeviceUID != "" {
		uid := report.InputDeviceUID
		m.ReportedInputDeviceUID = &uid
	}
	if report.ErrorCode != "" {
		m.ErrorCode = report.ErrorCode
		m.ErrorDetail = report.ErrorDetail
	}
	_, err = db.NewUpdate().Model(m).
		Column(
			"capabilities",
			"reported_at",
			"updated_at",
			"reported_output_device_uid",
			"reported_input_device_uid",
			"error_code",
			"error_detail",
		).
		WherePK().
		Exec(ctx)
	return err
}

func (s *ABCAudioStore) applyObservationReport(ctx context.Context, db bunIDB, m *abcAudioSettingsModel, report crosstalk.ABCAudioObservation) error {
	capsJSON := m.Capabilities
	if len(report.Capabilities) > 0 {
		b, err := marshalCapabilities(report.Capabilities)
		if err != nil {
			return err
		}
		capsJSON = b
	}
	now := report.ReportedAt.UTC()
	m.ReportedRevision = int64(report.DesiredRevision)
	if report.CommandID != "" {
		cid := report.CommandID
		m.ReportedCommandID = &cid
	}
	if report.OutputDeviceUID != "" {
		uid := report.OutputDeviceUID
		m.ReportedOutputDeviceUID = &uid
	}
	if report.InputDeviceUID != "" {
		uid := report.InputDeviceUID
		m.ReportedInputDeviceUID = &uid
	}
	m.ObservedOutputVolumePercent = int16PtrFromIntPtr(report.ObservedOutputVolumePercent)
	m.ObservedOutputMuted = report.ObservedOutputMuted
	m.ObservedInputGainPercent = int16PtrFromIntPtr(report.ObservedInputGainPercent)

	volState := report.OutputVolumeState
	muteState := report.OutputMuteState
	gainState := report.InputGainState
	if volState == "" {
		volState = crosstalk.ABCAudioControlUnknown
	}
	if muteState == "" {
		muteState = crosstalk.ABCAudioControlUnknown
	}
	if gainState == "" {
		gainState = crosstalk.ABCAudioControlUnknown
	}
	m.OutputVolumeState = string(volState)
	m.OutputMuteState = string(muteState)
	m.InputGainState = string(gainState)
	m.ErrorCode = report.ErrorCode
	m.ErrorDetail = report.ErrorDetail
	m.Capabilities = capsJSON
	m.ReportedAt = &now
	m.UpdatedAt = time.Now().UTC()

	_, err := db.NewUpdate().Model(m).
		Column(
			"reported_revision",
			"reported_command_id",
			"reported_output_device_uid",
			"observed_output_volume_percent",
			"observed_output_muted",
			"reported_input_device_uid",
			"observed_input_gain_percent",
			"output_volume_state",
			"output_mute_state",
			"input_gain_state",
			"error_code",
			"error_detail",
			"capabilities",
			"reported_at",
			"updated_at",
		).
		WherePK().
		Exec(ctx)
	return err
}

func (s *ABCAudioStore) statusFromModel(m *abcAudioSettingsModel, connected bool) *crosstalk.ABCAudioStatus {
	st := &crosstalk.ABCAudioStatus{
		ABCID:            m.ABCID,
		Connected:        connected,
		Desired:          desiredViewFromModel(m),
		Reported:         reportedViewFromModel(m),
		AcceptedRevision: uint64(m.DesiredRevision),
	}
	st.OverallState = crosstalk.DeriveABCAudioOverallState(*st)
	return st
}

// ---------------------------------------------------------------------------
// conversion helpers
// ---------------------------------------------------------------------------

type desiredDTO struct {
	Revision            uint64  `json:"revision"`
	CommandID           string  `json:"command_id,omitempty"`
	OutputDeviceUID     string  `json:"output_device_uid,omitempty"`
	OutputVolumePercent *int    `json:"output_volume_percent,omitempty"`
	OutputMuted         *bool   `json:"output_muted,omitempty"`
	InputDeviceUID      string  `json:"input_device_uid,omitempty"`
	InputGainPercent    *int    `json:"input_gain_percent,omitempty"`
	UpdatedAt           *string `json:"updated_at,omitempty"`
}

func desiredViewDTO(v crosstalk.ABCAudioDesiredView) desiredDTO {
	d := desiredDTO{
		Revision:            v.Revision,
		CommandID:           v.CommandID,
		OutputDeviceUID:     v.OutputDeviceUID,
		OutputVolumePercent: v.OutputVolumePercent,
		OutputMuted:         v.OutputMuted,
		InputDeviceUID:      v.InputDeviceUID,
		InputGainPercent:    v.InputGainPercent,
	}
	if v.UpdatedAt != nil {
		s := v.UpdatedAt.UTC().Format(time.RFC3339Nano)
		d.UpdatedAt = &s
	}
	return d
}

func desiredViewFromDTO(d desiredDTO) crosstalk.ABCAudioDesiredView {
	v := crosstalk.ABCAudioDesiredView{
		Revision:            d.Revision,
		CommandID:           d.CommandID,
		OutputDeviceUID:     d.OutputDeviceUID,
		OutputVolumePercent: d.OutputVolumePercent,
		OutputMuted:         d.OutputMuted,
		InputDeviceUID:      d.InputDeviceUID,
		InputGainPercent:    d.InputGainPercent,
	}
	if d.UpdatedAt != nil && *d.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339Nano, *d.UpdatedAt); err == nil {
			tt := t.UTC()
			v.UpdatedAt = &tt
		}
	}
	return v
}

func desiredViewFromModel(m *abcAudioSettingsModel) crosstalk.ABCAudioDesiredView {
	v := crosstalk.ABCAudioDesiredView{
		Revision: uint64(m.DesiredRevision),
	}
	if m.CommandID != nil {
		v.CommandID = *m.CommandID
	}
	if m.DesiredOutputDeviceUID != nil {
		v.OutputDeviceUID = *m.DesiredOutputDeviceUID
	}
	if m.DesiredInputDeviceUID != nil {
		v.InputDeviceUID = *m.DesiredInputDeviceUID
	}
	v.OutputVolumePercent = intPtrFromInt16Ptr(m.DesiredOutputVolumePercent)
	v.OutputMuted = m.DesiredOutputMuted
	v.InputGainPercent = intPtrFromInt16Ptr(m.DesiredInputGainPercent)
	if m.DesiredUpdatedAt != nil {
		t := m.DesiredUpdatedAt.UTC()
		v.UpdatedAt = &t
	}
	return v
}

func desiredFromModel(m *abcAudioSettingsModel) (crosstalk.ABCAudioDesired, bool) {
	if m.DesiredRevision == 0 ||
		m.DesiredOutputDeviceUID == nil ||
		m.DesiredInputDeviceUID == nil ||
		m.DesiredOutputVolumePercent == nil ||
		m.DesiredOutputMuted == nil ||
		m.DesiredInputGainPercent == nil {
		return crosstalk.ABCAudioDesired{}, false
	}
	return crosstalk.ABCAudioDesired{
		OutputDeviceUID:     *m.DesiredOutputDeviceUID,
		OutputVolumePercent: int(*m.DesiredOutputVolumePercent),
		OutputMuted:         *m.DesiredOutputMuted,
		InputDeviceUID:      *m.DesiredInputDeviceUID,
		InputGainPercent:    int(*m.DesiredInputGainPercent),
	}, true
}

func reportedViewFromModel(m *abcAudioSettingsModel) crosstalk.ABCAudioReportedView {
	v := crosstalk.ABCAudioReportedView{
		Revision:          uint64(m.ReportedRevision),
		OutputVolumeState: crosstalk.ABCAudioControlState(m.OutputVolumeState),
		OutputMuteState:   crosstalk.ABCAudioControlState(m.OutputMuteState),
		InputGainState:    crosstalk.ABCAudioControlState(m.InputGainState),
		ErrorCode:         m.ErrorCode,
		ErrorDetail:       m.ErrorDetail,
	}
	if m.ReportedCommandID != nil {
		v.CommandID = *m.ReportedCommandID
	}
	if m.ReportedOutputDeviceUID != nil {
		v.OutputDeviceUID = *m.ReportedOutputDeviceUID
	}
	if m.ReportedInputDeviceUID != nil {
		v.InputDeviceUID = *m.ReportedInputDeviceUID
	}
	v.ObservedOutputVolumePercent = intPtrFromInt16Ptr(m.ObservedOutputVolumePercent)
	v.ObservedOutputMuted = m.ObservedOutputMuted
	v.ObservedInputGainPercent = intPtrFromInt16Ptr(m.ObservedInputGainPercent)
	if m.ReportedAt != nil {
		t := m.ReportedAt.UTC()
		v.ReportedAt = &t
	}
	caps, _ := unmarshalCapabilities(m.Capabilities)
	v.Capabilities = caps
	return v
}

func auditToDomain(m *abcAudioAuditModel) (crosstalk.ABCAudioAuditEvent, error) {
	var prevDTO, nextDTO desiredDTO
	if err := json.Unmarshal(m.PreviousDesired, &prevDTO); err != nil {
		return crosstalk.ABCAudioAuditEvent{}, err
	}
	if err := json.Unmarshal(m.NewDesired, &nextDTO); err != nil {
		return crosstalk.ABCAudioAuditEvent{}, err
	}
	return crosstalk.ABCAudioAuditEvent{
		ID:              m.ID,
		ABCID:           m.ABCID,
		RequestID:       m.RequestID,
		ActorUserID:     m.ActorUserID,
		ActorRole:       m.ActorRole,
		DesiredRevision: uint64(m.DesiredRevision),
		PreviousDesired: desiredViewFromDTO(prevDTO),
		NewDesired:      desiredViewFromDTO(nextDTO),
		Outcome:         crosstalk.ABCAudioAuditOutcome(m.Outcome),
		CreatedAt:       m.CreatedAt.UTC(),
	}, nil
}

type capabilitiesEnvelope struct {
	Devices []crosstalk.ABCAudioCapability `json:"devices"`
}

func marshalCapabilities(caps []crosstalk.ABCAudioCapability) (json.RawMessage, error) {
	if caps == nil {
		caps = []crosstalk.ABCAudioCapability{}
	}
	b, err := json.Marshal(capabilitiesEnvelope{Devices: caps})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

func unmarshalCapabilities(b json.RawMessage) ([]crosstalk.ABCAudioCapability, error) {
	if len(b) == 0 || string(b) == "{}" || string(b) == "null" {
		return nil, nil
	}
	// Prefer envelope form; also accept a bare array for flexibility.
	var env capabilitiesEnvelope
	if err := json.Unmarshal(b, &env); err == nil && (env.Devices != nil || jsonHasKey(b, "devices")) {
		return env.Devices, nil
	}
	var arr []crosstalk.ABCAudioCapability
	if err := json.Unmarshal(b, &arr); err == nil {
		return arr, nil
	}
	return nil, nil
}

func jsonHasKey(b []byte, key string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

func intPtrFromInt16Ptr(p *int16) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}

func int16PtrFromIntPtr(p *int) *int16 {
	if p == nil {
		return nil
	}
	v := int16(*p)
	return &v
}

func desiredContentEqual(a, b crosstalk.ABCAudioDesired) bool {
	return a.OutputDeviceUID == b.OutputDeviceUID &&
		a.OutputVolumePercent == b.OutputVolumePercent &&
		a.OutputMuted == b.OutputMuted &&
		a.InputDeviceUID == b.InputDeviceUID &&
		a.InputGainPercent == b.InputGainPercent
}

func validateSetDesiredInput(actorID, actorRole, requestID string, desired crosstalk.ABCAudioDesired) error {
	if actorID == "" {
		return fmt.Errorf("%w: actor required", crosstalk.ErrABCAudioInvalidDesired)
	}
	if actorRole == "" {
		return fmt.Errorf("%w: actor role required", crosstalk.ErrABCAudioInvalidDesired)
	}
	if requestID == "" {
		return fmt.Errorf("%w: request_id required", crosstalk.ErrABCAudioInvalidDesired)
	}
	if len(requestID) > crosstalk.MaxABCAudioRequestIDLen {
		return fmt.Errorf("%w: request_id too long", crosstalk.ErrABCAudioInvalidDesired)
	}
	if err := crosstalk.ValidateABCAudioDesired(desired); err != nil {
		return err
	}
	return nil
}

func validateReport(report crosstalk.ABCAudioObservation) error {
	// Control states: empty allowed (treated as unknown); non-empty must be valid.
	for _, st := range []crosstalk.ABCAudioControlState{
		report.OutputVolumeState, report.OutputMuteState, report.InputGainState,
	} {
		if st == "" {
			continue
		}
		if !crosstalk.ValidABCAudioControlState(st) {
			return fmt.Errorf("%w: invalid control state %q", crosstalk.ErrABCAudioInvalidReport, st)
		}
	}
	if report.ObservedOutputVolumePercent != nil && !crosstalk.ValidateABCAudioPercent(*report.ObservedOutputVolumePercent) {
		return fmt.Errorf("%w: observed output volume out of range", crosstalk.ErrABCAudioInvalidReport)
	}
	if report.ObservedInputGainPercent != nil && !crosstalk.ValidateABCAudioPercent(*report.ObservedInputGainPercent) {
		return fmt.Errorf("%w: observed input gain out of range", crosstalk.ErrABCAudioInvalidReport)
	}
	return nil
}
