package webrtc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventRing_PushAndSnapshot(t *testing.T) {
	ring := NewEventRing(5)

	// Push 3 events (under capacity).
	for i := 0; i < 3; i++ {
		ring.Push(MakeEvent("peer1", EventICEStateChange, map[string]string{"state": "new"}))
	}

	assert.Equal(t, 3, ring.Len())
	assert.Equal(t, 3, ring.Total())

	snap := ring.Snapshot()
	require.Len(t, snap, 3)
	for _, e := range snap {
		assert.Equal(t, "peer1", e.PeerID)
		assert.Equal(t, EventICEStateChange, e.Type)
	}
}

func TestEventRing_Wraps(t *testing.T) {
	ring := NewEventRing(3)

	// Push 5 events into a ring of size 3 — should keep last 3.
	for i := 0; i < 5; i++ {
		ring.Push(MakeEvent("peer1", EventICEStateChange, map[string]int{"seq": i}))
	}

	assert.Equal(t, 3, ring.Len())
	assert.Equal(t, 5, ring.Total())

	snap := ring.Snapshot()
	require.Len(t, snap, 3)

	// Verify chronological order: seq 2, 3, 4.
	for idx, e := range snap {
		var detail map[string]int
		require.NoError(t, json.Unmarshal(e.Detail, &detail))
		assert.Equal(t, idx+2, detail["seq"], "event at index %d should have seq %d", idx, idx+2)
	}
}

func TestEventRing_DefaultCapacity(t *testing.T) {
	ring := NewEventRing(0)
	assert.Equal(t, 0, ring.Len())

	// Should default to 200.
	for i := 0; i < 250; i++ {
		ring.Push(MakeEvent("p", EventICEStateChange, nil))
	}
	assert.Equal(t, 200, ring.Len())
}

func TestMakeEvent_Detail(t *testing.T) {
	e := MakeEvent("peer-abc", EventSDPOffer, map[string]string{
		"type":   "offer",
		"tracks": "2",
	})

	assert.Equal(t, "peer-abc", e.PeerID)
	assert.Equal(t, EventSDPOffer, e.Type)
	assert.WithinDuration(t, time.Now(), e.Timestamp, time.Second)

	var detail map[string]string
	require.NoError(t, json.Unmarshal(e.Detail, &detail))
	assert.Equal(t, "offer", detail["type"])
	assert.Equal(t, "2", detail["tracks"])
}

func TestMakeEvent_NilDetail(t *testing.T) {
	e := MakeEvent("peer-abc", EventConnectionClosed, nil)
	assert.Equal(t, json.RawMessage(`{}`), e.Detail)
}
