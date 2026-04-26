package display

import "sync"

// StatusSnapshot is a point-in-time copy of the display state, safe
// to read without locks. Passed to Render.
type StatusSnapshot struct {
	NetworkLink   string
	NetworkAddr   string
	NetworkUp     bool
	ServerURL     string
	ControlState  string
	Channels      []ChannelInfo
	SessionID     string
	SessionRole   string
	SessionActive bool
	VUIn          float64
	VUOut         float64
}

// Status holds all the state displayed on the LCD. All writes go
// through setter methods which hold the lock.
type Status struct {
	mu            sync.RWMutex
	networkLink   string
	networkAddr   string
	networkUp     bool
	serverURL     string
	controlState  string
	channels      []ChannelInfo
	sessionID     string
	sessionRole   string
	sessionActive bool
	vuIn          float64
	vuOut         float64
}

// ChannelInfo describes one bound audio channel.
type ChannelInfo struct {
	ID        string
	Direction string // "SOURCE" or "SINK"
	Codec     string
	State     string // "active", "binding", "error", "idle"
}

// Snapshot returns a point-in-time copy for rendering.
func (s *Status) Snapshot() StatusSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ch := make([]ChannelInfo, len(s.channels))
	copy(ch, s.channels)
	return StatusSnapshot{
		NetworkLink:   s.networkLink,
		NetworkAddr:   s.networkAddr,
		NetworkUp:     s.networkUp,
		ServerURL:     s.serverURL,
		ControlState:  s.controlState,
		Channels:      ch,
		SessionID:     s.sessionID,
		SessionRole:   s.sessionRole,
		SessionActive: s.sessionActive,
		VUIn:          s.vuIn,
		VUOut:         s.vuOut,
	}
}

// SetNetwork updates network status fields.
func (s *Status) SetNetwork(link, addr string, up bool) {
	s.mu.Lock()
	s.networkLink = link
	s.networkAddr = addr
	s.networkUp = up
	s.mu.Unlock()
}

// SetServer updates server connection fields.
func (s *Status) SetServer(url, controlState string) {
	s.mu.Lock()
	s.serverURL = url
	s.controlState = controlState
	s.mu.Unlock()
}

// SetControlState updates only the control channel state.
func (s *Status) SetControlState(state string) {
	s.mu.Lock()
	s.controlState = state
	s.mu.Unlock()
}

// SetChannels replaces the channel list.
func (s *Status) SetChannels(ch []ChannelInfo) {
	s.mu.Lock()
	s.channels = ch
	s.mu.Unlock()
}

// UpsertChannel adds or updates a channel by ID.
func (s *Status) UpsertChannel(info ChannelInfo) {
	s.mu.Lock()
	for i, c := range s.channels {
		if c.ID == info.ID {
			s.channels[i] = info
			s.mu.Unlock()
			return
		}
	}
	s.channels = append(s.channels, info)
	s.mu.Unlock()
}

// RemoveChannel removes a channel by ID.
func (s *Status) RemoveChannel(id string) {
	s.mu.Lock()
	for i, c := range s.channels {
		if c.ID == id {
			s.channels = append(s.channels[:i], s.channels[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
}

// SetSession updates session fields.
func (s *Status) SetSession(id, role string, active bool) {
	s.mu.Lock()
	s.sessionID = id
	s.sessionRole = role
	s.sessionActive = active
	s.mu.Unlock()
}

// SetVU updates VU meter levels.
func (s *Status) SetVU(in, out float64) {
	s.mu.Lock()
	s.vuIn = in
	s.vuOut = out
	s.mu.Unlock()
}
