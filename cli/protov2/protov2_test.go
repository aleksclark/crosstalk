package protov2_test

import (
	"testing"

	"github.com/aleksclark/crosstalk/cli/protov2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildAndParseHelloV2(t *testing.T) {
	data := protov2.BuildHelloV2("abc", "Booth-A")
	require.NotEmpty(t, data)

	msg, err := protov2.ParseControlMessageV2(data)
	require.NoError(t, err)
	assert.Equal(t, protov2.PayloadHello, msg.Type)
}

func TestBuildAndParseWelcomeV2(t *testing.T) {
	// Manually encode a Welcome at field 10 of ControlMessage.
	// Welcome: peer_id=1, server_version=2, assigned_session_id=3
	welcome := encodeTestWelcome("peer-123", "1.0.0", "session-456")

	msg, err := protov2.ParseControlMessageV2(welcome)
	require.NoError(t, err)
	assert.Equal(t, protov2.PayloadWelcome, msg.Type)
	require.NotNil(t, msg.Welcome)
	assert.Equal(t, "peer-123", msg.Welcome.PeerID)
	assert.Equal(t, "1.0.0", msg.Welcome.ServerVersion)
	assert.Equal(t, "session-456", msg.Welcome.AssignedSessionID)
}

func TestBuildAndParseRestartV2(t *testing.T) {
	restart := encodeTestRestart("session reassigned")

	msg, err := protov2.ParseControlMessageV2(restart)
	require.NoError(t, err)
	assert.Equal(t, protov2.PayloadRestart, msg.Type)
	require.NotNil(t, msg.Restart)
	assert.Equal(t, "session reassigned", msg.Restart.Reason)
}

func TestBuildAndParseSessionAssignmentV2(t *testing.T) {
	sa := encodeTestSessionAssignment("session-789", "abc")

	msg, err := protov2.ParseControlMessageV2(sa)
	require.NoError(t, err)
	assert.Equal(t, protov2.PayloadSessionAssignment, msg.Type)
	require.NotNil(t, msg.SessionAssignment)
	assert.Equal(t, "session-789", msg.SessionAssignment.SessionID)
	assert.Equal(t, "abc", msg.SessionAssignment.Role)
}

func TestParseEmptyMessage(t *testing.T) {
	msg, err := protov2.ParseControlMessageV2([]byte{})
	require.NoError(t, err)
	assert.Equal(t, protov2.PayloadUnknown, msg.Type)
}

func TestParseInvalidData(t *testing.T) {
	// Invalid protobuf (truncated varint).
	_, err := protov2.ParseControlMessageV2([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	require.Error(t, err)
}

func TestBuildSourceStatusV2(t *testing.T) {
	data := protov2.BuildSourceStatusV2("mic1", true, 0.85)
	require.NotEmpty(t, data)

	msg, err := protov2.ParseControlMessageV2(data)
	require.NoError(t, err)
	assert.Equal(t, protov2.PayloadSourceStatus, msg.Type)
}

// --- Test helpers: manually encode v2 messages ---

func encodeTestWelcome(peerID, serverVersion, assignedSession string) []byte {
	// Build Welcome inner message.
	var inner []byte
	inner = appendTestString(inner, 1, peerID)
	inner = appendTestString(inner, 2, serverVersion)
	if assignedSession != "" {
		inner = appendTestString(inner, 3, assignedSession)
	}
	// Wrap in ControlMessage field 10.
	var msg []byte
	msg = appendTestBytes(msg, 10, inner)
	return msg
}

func encodeTestRestart(reason string) []byte {
	var inner []byte
	inner = appendTestString(inner, 1, reason)
	var msg []byte
	msg = appendTestBytes(msg, 12, inner)
	return msg
}

func encodeTestSessionAssignment(sessionID, role string) []byte {
	var inner []byte
	inner = appendTestString(inner, 1, sessionID)
	inner = appendTestString(inner, 2, role)
	var msg []byte
	msg = appendTestBytes(msg, 13, inner)
	return msg
}

func appendTestString(data []byte, fieldNum int, s string) []byte {
	return appendTestBytes(data, fieldNum, []byte(s))
}

func appendTestBytes(data []byte, fieldNum int, value []byte) []byte {
	tag := uint64(fieldNum<<3) | 2
	data = appendTestVarint(data, tag)
	data = appendTestVarint(data, uint64(len(value)))
	data = append(data, value...)
	return data
}

func appendTestVarint(data []byte, v uint64) []byte {
	for v >= 0x80 {
		data = append(data, byte(v)|0x80)
		v >>= 7
	}
	data = append(data, byte(v))
	return data
}
