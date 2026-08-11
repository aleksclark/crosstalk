package httpapi

import (
	"context"

	"github.com/aleksclark/crosstalk/cli/play"
)

// Ensure Client satisfies play.API via Adapter.
var _ play.API = (*Adapter)(nil)

// Adapter maps httpapi types onto play domain types.
type Adapter struct {
	Client *Client
}

// NewAdapter wraps a Client as play.API.
func NewAdapter(c *Client) *Adapter {
	return &Adapter{Client: c}
}

func (a *Adapter) Login(ctx context.Context, username, password string) (string, error) {
	return a.Client.Login(ctx, username, password)
}

func (a *Adapter) ListSessions(ctx context.Context, accessToken string) ([]play.Session, error) {
	rows, err := a.Client.ListSessions(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	out := make([]play.Session, len(rows))
	for i, r := range rows {
		out[i] = play.Session{ID: r.ID, Name: r.Name}
	}
	return out, nil
}

func (a *Adapter) ListChannels(ctx context.Context, accessToken, sessionID string) ([]play.Channel, error) {
	rows, err := a.Client.ListChannels(ctx, accessToken, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]play.Channel, len(rows))
	for i, r := range rows {
		out[i] = play.Channel{ID: r.ID, SessionID: r.SessionID, Name: r.Name, Type: r.Type}
	}
	return out, nil
}

func (a *Adapter) IssueMediaTicket(ctx context.Context, accessToken, sessionID string, produceChannelIDs []string) (*play.MediaTicket, error) {
	t, err := a.Client.IssueMediaTicket(ctx, accessToken, sessionID, produceChannelIDs)
	if err != nil {
		return nil, err
	}
	return &play.MediaTicket{
		Token:             t.Token,
		ExpiresAt:         t.ExpiresAt,
		SessionID:         t.SessionID,
		Role:              t.Role,
		ProduceChannelIDs: append([]string(nil), t.ProduceChannelIDs...),
		ListenChannelIDs:  append([]string(nil), t.ListenChannelIDs...),
		OwnerGeneration:   t.OwnerGeneration,
	}, nil
}
