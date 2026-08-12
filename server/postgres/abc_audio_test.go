package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun/driver/pgdriver"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/pgtest"
	"github.com/aleksclark/crosstalk/server/postgres"
)

func seedABC(t *testing.T, db *postgres.DB) *crosstalk.ABC {
	t.Helper()
	store := postgres.NewABCStore(db)
	abc := &crosstalk.ABC{
		ID:        ulid.Make().String(),
		Name:      "Booth Audio",
		TokenHash: "tok-" + ulid.Make().String(),
	}
	require.NoError(t, store.Create(context.Background(), abc))
	return abc
}

func sampleDesired(vol, gain int, muted bool) crosstalk.ABCAudioDesired {
	return crosstalk.ABCAudioDesired{
		OutputDeviceUID:     "usb:0d8c:0014:path:platform-xhci-0_1",
		OutputVolumePercent: vol,
		OutputMuted:         muted,
		InputDeviceUID:      "usb:0d8c:0014:path:platform-xhci-0_1",
		InputGainPercent:    gain,
	}
}

func sampleCaps() []crosstalk.ABCAudioCapability {
	return []crosstalk.ABCAudioCapability{
		{
			DeviceUID:      "usb:0d8c:0014:path:platform-xhci-0_1",
			Direction:      "both",
			Backend:        "alsa",
			SupportsVolume: true,
			SupportsMute:   true,
			SupportsGain:   true,
		},
	}
}

func intPtr(n int) *int    { return &n }
func boolPtr(b bool) *bool { return &b }

func TestABCAudio_UnconfiguredGet(t *testing.T) {
	db := pgtest.New(t)
	abc := seedABC(t, db)
	store := postgres.NewABCAudioStore(db)
	ctx := context.Background()

	st, err := store.Get(ctx, abc.ID)
	require.NoError(t, err)
	require.NotNil(t, st)
	assert.Equal(t, abc.ID, st.ABCID)
	assert.Equal(t, uint64(0), st.Desired.Revision)
	assert.Equal(t, uint64(0), st.Reported.Revision)
	assert.Equal(t, crosstalk.ABCAudioOverallUnconfigured, st.OverallState)
	assert.Equal(t, crosstalk.ABCAudioControlUnknown, st.Reported.OutputVolumeState)
	assert.Nil(t, st.Desired.OutputVolumePercent)
	assert.Empty(t, st.Desired.OutputDeviceUID)

	// Get must not create a row.
	var n int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM abc_audio_settings WHERE abc_id = ?`, abc.ID).Scan(&n))
	assert.Equal(t, 0, n)

	_, err = store.Get(ctx, "missing-abc")
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrABCAudioABCNotFound), "%v", err)
}

func TestABCAudio_FirstRevisionAndAbsoluteReplacement(t *testing.T) {
	db := pgtest.New(t)
	abc := seedABC(t, db)
	store := postgres.NewABCAudioStore(db)
	ctx := context.Background()

	d1 := sampleDesired(65, 40, false)
	st, err := store.SetDesired(ctx, abc.ID, "user-1", "admin", "req-1", 0, d1)
	require.NoError(t, err)
	require.NotNil(t, st)
	assert.Equal(t, uint64(1), st.Desired.Revision)
	assert.Equal(t, uint64(1), st.AcceptedRevision)
	assert.Equal(t, crosstalk.ABCAudioCommandID(abc.ID, 1), st.Desired.CommandID)
	require.NotNil(t, st.Desired.OutputVolumePercent)
	assert.Equal(t, 65, *st.Desired.OutputVolumePercent)
	assert.Equal(t, false, *st.Desired.OutputMuted)
	assert.Equal(t, 40, *st.Desired.InputGainPercent)
	assert.Equal(t, crosstalk.ABCAudioControlPending, st.Reported.OutputVolumeState)
	assert.Equal(t, crosstalk.ABCAudioOverallOffline, st.OverallState) // not connected by default

	// Absolute replacement at expected=1.
	d2 := sampleDesired(10, 90, true)
	st2, err := store.SetDesired(ctx, abc.ID, "user-1", "admin", "req-2", 1, d2)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), st2.Desired.Revision)
	assert.Equal(t, 10, *st2.Desired.OutputVolumePercent)
	assert.Equal(t, true, *st2.Desired.OutputMuted)
	assert.Equal(t, 90, *st2.Desired.InputGainPercent)
	assert.Equal(t, crosstalk.ABCAudioCommandID(abc.ID, 2), st2.Desired.CommandID)

	// Desired survives reopen.
	log := db // already open; re-Get is enough
	_ = log
	got, err := store.Get(ctx, abc.ID)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), got.Desired.Revision)
	assert.Equal(t, 10, *got.Desired.OutputVolumePercent)
}

func TestABCAudio_NoOpEqualDesired(t *testing.T) {
	db := pgtest.New(t)
	abc := seedABC(t, db)
	store := postgres.NewABCAudioStore(db)
	ctx := context.Background()

	d := sampleDesired(50, 50, false)
	_, err := store.SetDesired(ctx, abc.ID, "u", "admin", "r1", 0, d)
	require.NoError(t, err)

	st, err := store.SetDesired(ctx, abc.ID, "u", "admin", "r2", 1, d)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), st.Desired.Revision, "no-op must not bump revision")
	assert.Equal(t, uint64(1), st.AcceptedRevision)

	events, err := store.ListAudit(ctx, abc.ID, 10)
	require.NoError(t, err)
	require.Len(t, events, 2)
	// newest first
	assert.Equal(t, crosstalk.ABCAudioAuditNoOp, events[0].Outcome)
	assert.Equal(t, uint64(1), events[0].DesiredRevision)
	assert.Equal(t, crosstalk.ABCAudioAuditAccepted, events[1].Outcome)
	assert.Equal(t, uint64(1), events[1].DesiredRevision)
}

func TestABCAudio_DuplicateRequestID(t *testing.T) {
	db := pgtest.New(t)
	abc := seedABC(t, db)
	store := postgres.NewABCAudioStore(db)
	ctx := context.Background()

	d1 := sampleDesired(20, 30, false)
	st1, err := store.SetDesired(ctx, abc.ID, "u", "admin", "same-req", 0, d1)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), st1.Desired.Revision)

	// Same request_id with different desired must return original, not create rev 2.
	d2 := sampleDesired(99, 1, true)
	st2, err := store.SetDesired(ctx, abc.ID, "u", "admin", "same-req", 0, d2)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), st2.Desired.Revision)
	assert.Equal(t, 20, *st2.Desired.OutputVolumePercent)

	events, err := store.ListAudit(ctx, abc.ID, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
}

func TestABCAudio_RevisionConflict(t *testing.T) {
	db := pgtest.New(t)
	abc := seedABC(t, db)
	store := postgres.NewABCAudioStore(db)
	ctx := context.Background()

	_, err := store.SetDesired(ctx, abc.ID, "u", "admin", "r1", 0, sampleDesired(1, 1, false))
	require.NoError(t, err)

	_, err = store.SetDesired(ctx, abc.ID, "u", "admin", "r2", 0, sampleDesired(2, 2, false))
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrABCAudioRevisionConflict), "%v", err)

	// Wrong future expected.
	_, err = store.SetDesired(ctx, abc.ID, "u", "admin", "r3", 5, sampleDesired(3, 3, false))
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrABCAudioRevisionConflict), "%v", err)

	got, err := store.Get(ctx, abc.ID)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), got.Desired.Revision)
}

func TestABCAudio_AuditActorAndSnapshots(t *testing.T) {
	db := pgtest.New(t)
	abc := seedABC(t, db)
	store := postgres.NewABCAudioStore(db)
	ctx := context.Background()

	d1 := sampleDesired(10, 20, false)
	_, err := store.SetDesired(ctx, abc.ID, "actor-A", "admin", "a1", 0, d1)
	require.NoError(t, err)
	d2 := sampleDesired(11, 21, true)
	_, err = store.SetDesired(ctx, abc.ID, "actor-B", "admin", "a2", 1, d2)
	require.NoError(t, err)

	events, err := store.ListAudit(ctx, abc.ID, 10)
	require.NoError(t, err)
	require.Len(t, events, 2)

	// newest = a2
	assert.Equal(t, "actor-B", events[0].ActorUserID)
	assert.Equal(t, "admin", events[0].ActorRole)
	assert.Equal(t, "a2", events[0].RequestID)
	assert.Equal(t, uint64(2), events[0].DesiredRevision)
	assert.Equal(t, crosstalk.ABCAudioAuditAccepted, events[0].Outcome)
	require.NotNil(t, events[0].PreviousDesired.OutputVolumePercent)
	assert.Equal(t, 10, *events[0].PreviousDesired.OutputVolumePercent)
	require.NotNil(t, events[0].NewDesired.OutputVolumePercent)
	assert.Equal(t, 11, *events[0].NewDesired.OutputVolumePercent)

	assert.Equal(t, "actor-A", events[1].ActorUserID)
	assert.Equal(t, uint64(0), events[1].PreviousDesired.Revision)
	assert.Equal(t, uint64(1), events[1].NewDesired.Revision)
}

func TestABCAudio_PercentAndStateConstraints(t *testing.T) {
	db := pgtest.New(t)
	abc := seedABC(t, db)
	store := postgres.NewABCAudioStore(db)
	ctx := context.Background()

	_, err := store.SetDesired(ctx, abc.ID, "u", "admin", "bad-vol", 0, sampleDesired(101, 50, false))
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrABCAudioInvalidDesired), "%v", err)

	_, err = store.SetDesired(ctx, abc.ID, "u", "admin", "bad-gain", 0, sampleDesired(50, -1, false))
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrABCAudioInvalidDesired), "%v", err)

	// Empty request_id
	_, err = store.SetDesired(ctx, abc.ID, "u", "admin", "", 0, sampleDesired(50, 50, false))
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrABCAudioInvalidDesired), "%v", err)

	// Overlong request_id
	_, err = store.SetDesired(ctx, abc.ID, "u", "admin", strings.Repeat("x", 65), 0, sampleDesired(50, 50, false))
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrABCAudioInvalidDesired), "%v", err)

	// DB CHECK: invalid control state rejected.
	_, err = store.SetDesired(ctx, abc.ID, "u", "admin", "ok", 0, sampleDesired(0, 100, false))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		UPDATE abc_audio_settings SET output_volume_state = 'bogus' WHERE abc_id = ?
	`, abc.ID)
	require.Error(t, err)

	// DB CHECK: invalid percent rejected.
	_, err = db.ExecContext(ctx, `
		UPDATE abc_audio_settings SET desired_output_volume_percent = 101 WHERE abc_id = ?
	`, abc.ID)
	require.Error(t, err)

	// Boundaries 0 and 100 accepted.
	st, err := store.SetDesired(ctx, abc.ID, "u", "admin", "bounds", 1, sampleDesired(0, 100, true))
	require.NoError(t, err)
	assert.Equal(t, 0, *st.Desired.OutputVolumePercent)
	assert.Equal(t, 100, *st.Desired.InputGainPercent)
}

func TestABCAudio_ReportsStaleEqualAndOutOfOrder(t *testing.T) {
	db := pgtest.New(t)
	abc := seedABC(t, db)
	store := postgres.NewABCAudioStore(db)
	ctx := context.Background()

	// First inventory (rev 0) may create row + caps without desired.
	inv := crosstalk.ABCAudioObservation{
		DesiredRevision:   0,
		Capabilities:      sampleCaps(),
		OutputVolumeState: crosstalk.ABCAudioControlUnknown,
		OutputMuteState:   crosstalk.ABCAudioControlUnknown,
		InputGainState:    crosstalk.ABCAudioControlUnknown,
		ReportedAt:        time.Now().UTC(),
	}
	st, err := store.RecordReport(ctx, abc.ID, inv)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), st.Desired.Revision)
	assert.Equal(t, uint64(0), st.Reported.Revision)
	require.Len(t, st.Reported.Capabilities, 1)
	assert.Equal(t, "usb:0d8c:0014:path:platform-xhci-0_1", st.Reported.Capabilities[0].DeviceUID)

	// Admin save → rev 1
	d := sampleDesired(65, 40, false)
	st, err = store.SetDesired(ctx, abc.ID, "u", "admin", "save1", 0, d)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), st.Desired.Revision)
	// Desired must not be wiped by prior inventory.
	assert.Equal(t, 65, *st.Desired.OutputVolumePercent)

	// Applied report for rev 1
	vol, gain := 64, 40
	muted := false
	applied := crosstalk.ABCAudioObservation{
		DesiredRevision:             1,
		CommandID:                   crosstalk.ABCAudioCommandID(abc.ID, 1),
		OutputDeviceUID:             d.OutputDeviceUID,
		ObservedOutputVolumePercent: &vol,
		ObservedOutputMuted:         &muted,
		InputDeviceUID:              d.InputDeviceUID,
		ObservedInputGainPercent:    &gain,
		OutputVolumeState:           crosstalk.ABCAudioControlApplied,
		OutputMuteState:             crosstalk.ABCAudioControlApplied,
		InputGainState:              crosstalk.ABCAudioControlApplied,
		Capabilities:                sampleCaps(),
		ReportedAt:                  time.Now().UTC(),
	}
	st, err = store.RecordReport(ctx, abc.ID, applied)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), st.Reported.Revision)
	assert.Equal(t, 64, *st.Reported.ObservedOutputVolumePercent)
	// Observed never changes desired.
	assert.Equal(t, 65, *st.Desired.OutputVolumePercent)

	// Equal-revision re-report updates observed readback.
	vol2 := 65
	applied2 := applied
	applied2.ObservedOutputVolumePercent = &vol2
	applied2.ReportedAt = time.Now().UTC().Add(time.Second)
	st, err = store.RecordReport(ctx, abc.ID, applied2)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), st.Reported.Revision)
	assert.Equal(t, 65, *st.Reported.ObservedOutputVolumePercent)

	// Advance desired to 2, then apply report for 2.
	_, err = store.SetDesired(ctx, abc.ID, "u", "admin", "save2", 1, sampleDesired(70, 45, true))
	require.NoError(t, err)
	vol3, gain3 := 70, 45
	muted3 := true
	applied3 := crosstalk.ABCAudioObservation{
		DesiredRevision:             2,
		CommandID:                   crosstalk.ABCAudioCommandID(abc.ID, 2),
		OutputDeviceUID:             d.OutputDeviceUID,
		ObservedOutputVolumePercent: &vol3,
		ObservedOutputMuted:         &muted3,
		InputDeviceUID:              d.InputDeviceUID,
		ObservedInputGainPercent:    &gain3,
		OutputVolumeState:           crosstalk.ABCAudioControlApplied,
		OutputMuteState:             crosstalk.ABCAudioControlApplied,
		InputGainState:              crosstalk.ABCAudioControlApplied,
		ReportedAt:                  time.Now().UTC(),
	}
	st, err = store.RecordReport(ctx, abc.ID, applied3)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), st.Reported.Revision)

	// Stale/out-of-order report (rev 1) must not roll back observed revision.
	stale := applied
	stale.DesiredRevision = 1
	stale.ObservedOutputVolumePercent = intPtr(1)
	st, err = store.RecordReport(ctx, abc.ID, stale)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), st.Reported.Revision)
	assert.Equal(t, 70, *st.Reported.ObservedOutputVolumePercent)
	assert.Equal(t, uint64(2), st.Desired.Revision)
	assert.Equal(t, 70, *st.Desired.OutputVolumePercent)

	// Inventory-only rev 0 after configured desired must update caps only.
	inv2 := crosstalk.ABCAudioObservation{
		DesiredRevision: 0,
		Capabilities: []crosstalk.ABCAudioCapability{
			{DeviceUID: "usb:dead:beef:path:other", Direction: "output", Backend: "alsa"},
		},
		ReportedAt: time.Now().UTC(),
	}
	st, err = store.RecordReport(ctx, abc.ID, inv2)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), st.Desired.Revision)
	assert.Equal(t, 70, *st.Desired.OutputVolumePercent)
	assert.Equal(t, uint64(2), st.Reported.Revision)
	require.Len(t, st.Reported.Capabilities, 1)
	assert.Equal(t, "usb:dead:beef:path:other", st.Reported.Capabilities[0].DeviceUID)
}

func TestABCAudio_ErrorDetailSanitizedAndCapped(t *testing.T) {
	db := pgtest.New(t)
	abc := seedABC(t, db)
	store := postgres.NewABCAudioStore(db)
	ctx := context.Background()

	_, err := store.SetDesired(ctx, abc.ID, "u", "admin", "s", 0, sampleDesired(1, 1, false))
	require.NoError(t, err)

	detail := strings.Repeat("a", 300) + "\x00\x01boom"
	rep := crosstalk.ABCAudioObservation{
		DesiredRevision:   1,
		CommandID:         crosstalk.ABCAudioCommandID(abc.ID, 1),
		OutputVolumeState: crosstalk.ABCAudioControlError,
		OutputMuteState:   crosstalk.ABCAudioControlError,
		InputGainState:    crosstalk.ABCAudioControlError,
		ErrorCode:         "APPLY_FAILED",
		ErrorDetail:       detail,
		ReportedAt:        time.Now().UTC(),
	}
	st, err := store.RecordReport(ctx, abc.ID, rep)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(st.Reported.ErrorDetail), crosstalk.MaxABCAudioErrorDetailBytes)
	assert.NotContains(t, st.Reported.ErrorDetail, "\x00")
	assert.Equal(t, "APPLY_FAILED", st.Reported.ErrorCode)
}

func TestABCAudio_CascadeDelete(t *testing.T) {
	db := pgtest.New(t)
	abc := seedABC(t, db)
	store := postgres.NewABCAudioStore(db)
	abcStore := postgres.NewABCStore(db)
	ctx := context.Background()

	_, err := store.SetDesired(ctx, abc.ID, "u", "admin", "c1", 0, sampleDesired(5, 5, false))
	require.NoError(t, err)
	_, err = store.RecordReport(ctx, abc.ID, crosstalk.ABCAudioObservation{
		DesiredRevision:   1,
		OutputVolumeState: crosstalk.ABCAudioControlApplied,
		OutputMuteState:   crosstalk.ABCAudioControlApplied,
		InputGainState:    crosstalk.ABCAudioControlApplied,
		ReportedAt:        time.Now().UTC(),
	})
	require.NoError(t, err)

	require.NoError(t, abcStore.Delete(ctx, abc.ID))

	var settingsN, auditN int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM abc_audio_settings WHERE abc_id = ?`, abc.ID).Scan(&settingsN))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM abc_audio_audit_events WHERE abc_id = ?`, abc.ID).Scan(&auditN))
	assert.Equal(t, 0, settingsN)
	assert.Equal(t, 0, auditN)
}

func TestABCAudio_ConcurrentSetDesiredOneRevision(t *testing.T) {
	db := pgtest.New(t)
	abc := seedABC(t, db)
	store := postgres.NewABCAudioStore(db)
	ctx := context.Background()

	const n = 12
	var wg sync.WaitGroup
	errs := make([]error, n)
	revs := make([]uint64, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			req := fmt.Sprintf("concurrent-%d", i)
			// All start from expected 0; only one should win revision 1 with unique request ids.
			// Different request ids + same expected → one accepts, others conflict.
			st, err := store.SetDesired(ctx, abc.ID, "u", "admin", req, 0, sampleDesired(i%100, i%100, false))
			errs[i] = err
			if err == nil && st != nil {
				revs[i] = st.Desired.Revision
			}
		}(i)
	}
	wg.Wait()

	accepted := 0
	conflicts := 0
	for i, err := range errs {
		if err == nil {
			accepted++
			assert.Equal(t, uint64(1), revs[i])
			continue
		}
		if errors.Is(err, crosstalk.ErrABCAudioRevisionConflict) {
			conflicts++
			continue
		}
		t.Fatalf("worker %d unexpected error: %v", i, err)
	}
	assert.Equal(t, 1, accepted)
	assert.Equal(t, n-1, conflicts)

	events, err := store.ListAudit(ctx, abc.ID, 100)
	require.NoError(t, err)
	require.Len(t, events, 1)

	got, err := store.Get(ctx, abc.ID)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), got.Desired.Revision)
}

func TestABCAudio_ConcurrentDuplicateRequestID(t *testing.T) {
	db := pgtest.New(t)
	abc := seedABC(t, db)
	store := postgres.NewABCAudioStore(db)
	ctx := context.Background()

	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)
	revs := make([]uint64, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			st, err := store.SetDesired(ctx, abc.ID, "u", "admin", "dup-req", 0, sampleDesired(42, 42, false))
			errs[i] = err
			if err == nil && st != nil {
				revs[i] = st.Desired.Revision
			}
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "worker %d", i)
		assert.Equal(t, uint64(1), revs[i])
	}
	events, err := store.ListAudit(ctx, abc.ID, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
}

func TestMigration4_CreatesAudioTablesAndNoBackfill(t *testing.T) {
	db := pgtest.New(t)
	ctx := context.Background()

	// Existing ABC without settings remains unconfigured (no backfill).
	abc := seedABC(t, db)
	var n int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM abc_audio_settings`).Scan(&n))
	assert.Equal(t, 0, n)

	var tableCount int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_name IN ('abc_audio_settings', 'abc_audio_audit_events')
	`).Scan(&tableCount))
	assert.Equal(t, 2, tableCount)

	var ver int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = 4`).Scan(&ver))
	assert.Equal(t, 1, ver)

	// Unconfigured get still works.
	store := postgres.NewABCAudioStore(db)
	st, err := store.Get(ctx, abc.ID)
	require.NoError(t, err)
	assert.Equal(t, crosstalk.ABCAudioOverallUnconfigured, st.OverallState)
}

func TestMigration4_ConcurrentOpenSafe(t *testing.T) {
	admin := pgtest.AdminDSN()
	dbName := "ct_migaudio_" + strings.ToLower(ulid.Make().String())
	adminDB := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(admin)))
	ctx := context.Background()
	_, err := adminDB.ExecContext(ctx, fmt.Sprintf(`CREATE DATABASE %q`, dbName))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = adminDB.ExecContext(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, dbName))
		_ = adminDB.Close()
	})
	dsn := rewriteDB(admin, dbName)

	const n = 6
	var wg sync.WaitGroup
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			d, err := postgres.Open(dsn, nil)
			if err != nil {
				errs[i] = err
				return
			}
			_ = d.Close()
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "worker %d", i)
	}

	check := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	defer func() { _ = check.Close() }()
	var count int
	require.NoError(t, check.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count))
	assert.Equal(t, len(postgres.Migrations()), count)
	var audioTables int
	require.NoError(t, check.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_name IN ('abc_audio_settings', 'abc_audio_audit_events')
	`).Scan(&audioTables))
	assert.Equal(t, 2, audioTables)
}

func TestABCAudio_InvalidReportStates(t *testing.T) {
	db := pgtest.New(t)
	abc := seedABC(t, db)
	store := postgres.NewABCAudioStore(db)
	ctx := context.Background()

	_, err := store.RecordReport(ctx, abc.ID, crosstalk.ABCAudioObservation{
		DesiredRevision:   1,
		OutputVolumeState: "not-a-state",
		OutputMuteState:   crosstalk.ABCAudioControlApplied,
		InputGainState:    crosstalk.ABCAudioControlApplied,
		ReportedAt:        time.Now().UTC(),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrABCAudioInvalidReport), "%v", err)

	// Invalid observed percent
	bad := 150
	_, err = store.RecordReport(ctx, abc.ID, crosstalk.ABCAudioObservation{
		DesiredRevision:             1,
		ObservedOutputVolumePercent: &bad,
		OutputVolumeState:           crosstalk.ABCAudioControlApplied,
		OutputMuteState:             crosstalk.ABCAudioControlApplied,
		InputGainState:              crosstalk.ABCAudioControlApplied,
		ReportedAt:                  time.Now().UTC(),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrABCAudioInvalidReport), "%v", err)
}

func TestABCAudio_DeriveOverallStateOrder(t *testing.T) {
	// Pure domain derivation — no DB.
	base := crosstalk.ABCAudioStatus{
		Desired:  crosstalk.ABCAudioDesiredView{Revision: 1},
		Reported: crosstalk.ABCAudioReportedView{Revision: 1},
	}

	u := base
	u.Desired.Revision = 0
	assert.Equal(t, crosstalk.ABCAudioOverallUnconfigured, crosstalk.DeriveABCAudioOverallState(u))

	off := base
	off.Connected = false
	assert.Equal(t, crosstalk.ABCAudioOverallOffline, crosstalk.DeriveABCAudioOverallState(off))

	stale := base
	stale.Connected = true
	stale.Stale = true
	assert.Equal(t, crosstalk.ABCAudioOverallStale, crosstalk.DeriveABCAudioOverallState(stale))

	pending := base
	pending.Connected = true
	pending.Reported.Revision = 0
	assert.Equal(t, crosstalk.ABCAudioOverallPending, crosstalk.DeriveABCAudioOverallState(pending))

	errS := base
	errS.Connected = true
	errS.Reported.OutputVolumeState = crosstalk.ABCAudioControlError
	errS.Reported.OutputMuteState = crosstalk.ABCAudioControlApplied
	errS.Reported.InputGainState = crosstalk.ABCAudioControlApplied
	assert.Equal(t, crosstalk.ABCAudioOverallError, crosstalk.DeriveABCAudioOverallState(errS))

	mm := base
	mm.Connected = true
	mm.Reported.OutputVolumeState = crosstalk.ABCAudioControlDeviceMismatch
	mm.Reported.OutputMuteState = crosstalk.ABCAudioControlApplied
	mm.Reported.InputGainState = crosstalk.ABCAudioControlApplied
	assert.Equal(t, crosstalk.ABCAudioOverallDeviceMismatch, crosstalk.DeriveABCAudioOverallState(mm))

	uns := base
	uns.Connected = true
	uns.Reported.OutputVolumeState = crosstalk.ABCAudioControlUnsupported
	uns.Reported.OutputMuteState = crosstalk.ABCAudioControlUnsupported
	uns.Reported.InputGainState = crosstalk.ABCAudioControlUnsupported
	assert.Equal(t, crosstalk.ABCAudioOverallUnsupported, crosstalk.DeriveABCAudioOverallState(uns))

	part := base
	part.Connected = true
	part.Reported.OutputVolumeState = crosstalk.ABCAudioControlApplied
	part.Reported.OutputMuteState = crosstalk.ABCAudioControlUnsupported
	part.Reported.InputGainState = crosstalk.ABCAudioControlApplied
	assert.Equal(t, crosstalk.ABCAudioOverallPartial, crosstalk.DeriveABCAudioOverallState(part))

	app := base
	app.Connected = true
	app.Reported.OutputVolumeState = crosstalk.ABCAudioControlApplied
	app.Reported.OutputMuteState = crosstalk.ABCAudioControlApplied
	app.Reported.InputGainState = crosstalk.ABCAudioControlApplied
	assert.Equal(t, crosstalk.ABCAudioOverallApplied, crosstalk.DeriveABCAudioOverallState(app))
}
