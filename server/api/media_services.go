package api

import (
	"context"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/mediaticket"
	"github.com/aleksclark/crosstalk/server/ownership"
)

// MediaTicketIssuer mints and consumes one-time media admission tickets.
// *mediaticket.Service implements this interface.
type MediaTicketIssuer interface {
	Issue(ctx context.Context, req mediaticket.IssueRequest) (*mediaticket.IssuedTicket, error)
	Consume(ctx context.Context, req mediaticket.ConsumeRequest) (*crosstalk.MediaTicket, error)
	ParseToken(tokenStr string) (*mediaticket.Claims, error)
}

// SessionLeaseService manages fenced session owner leases.
// ownership.Service / *ownership.Store implement this interface.
type SessionLeaseService interface {
	Acquire(ctx context.Context, sessionID, ownerID string, ttl time.Duration) (ownership.Lease, error)
	Renew(ctx context.Context, lease ownership.Lease, ttl time.Duration) (ownership.Lease, error)
	Release(ctx context.Context, lease ownership.Lease) error
	Current(ctx context.Context, sessionID string) (ownership.Lease, error)
}

// Compile-time checks against control-lane implementations.
var (
	_ MediaTicketIssuer   = (*mediaticket.Service)(nil)
	_ SessionLeaseService = (*ownership.Store)(nil)
	_ SessionLeaseService = (ownership.Service)(nil)
)
