// Package mediaticket issues and consumes one-time, session-scoped media
// admission tickets. Claims bind audience, session, owner generation, subject,
// role, and explicit channel IDs. Only a hash of the nonce/JTI is persisted.
package mediaticket

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oklog/ulid/v2"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// Audience is the fixed JWT audience for media tickets.
const Audience = "crosstalk-media"

// MaxTTL is the maximum allowed ticket lifetime.
const MaxTTL = 60 * time.Second

// Claims are the JWT claims carried by a media ticket.
type Claims struct {
	jwt.RegisteredClaims
	SessionID         string   `json:"session_id"`
	OwnerID           string   `json:"owner_id"`
	OwnerGeneration   uint64   `json:"owner_generation"`
	Role              string   `json:"role"`
	ProduceChannelIDs []string `json:"produce_channel_ids"`
	ListenChannelIDs  []string `json:"listen_channel_ids"`
}

// IssuedTicket is the result of a successful Issue call.
type IssuedTicket struct {
	// Token is a signed JWT (when a signing key is configured) or the opaque
	// nonce used as the one-time credential.
	Token string
	// Nonce is the plaintext one-time secret (also embedded as JTI).
	Nonce string
	// Ticket is the persisted metadata (nonce hashed).
	Ticket crosstalk.MediaTicket
	// Claims are the bound claim set.
	Claims Claims
}

// IssueRequest is the input for minting a media ticket.
type IssueRequest struct {
	SessionID         string
	OwnerID           string
	OwnerGeneration   uint64
	Subject           string
	Role              string
	ProduceChannelIDs []string
	ListenChannelIDs  []string
	TTL               time.Duration
}

// Service mints and consumes media tickets.
type Service struct {
	store     crosstalk.MediaTicketService
	signingKey []byte
	now       func() time.Time
}

// NewService constructs a media-ticket service.
// signingKey may be empty; then Issue returns the opaque nonce as Token.
func NewService(store crosstalk.MediaTicketService, signingKey []byte) *Service {
	return &Service{
		store:      store,
		signingKey: append([]byte(nil), signingKey...),
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// Issue validates claim constraints, persists a hashed nonce, and returns the
// one-time credential. Expiry is capped at MaxTTL.
func (s *Service) Issue(ctx context.Context, req IssueRequest) (*IssuedTicket, error) {
	if req.SessionID == "" || req.Subject == "" || req.Role == "" {
		return nil, fmt.Errorf("%w: session_id, subject, and role are required", crosstalk.ErrTicketInvalid)
	}
	if req.OwnerID == "" {
		return nil, fmt.Errorf("%w: owner_id is required", crosstalk.ErrTicketInvalid)
	}
	if req.TTL <= 0 {
		req.TTL = MaxTTL
	}
	if req.TTL > MaxTTL {
		return nil, fmt.Errorf("%w: ttl %s exceeds max %s", crosstalk.ErrTicketInvalid, req.TTL, MaxTTL)
	}
	if req.ProduceChannelIDs == nil {
		req.ProduceChannelIDs = []string{}
	}
	if req.ListenChannelIDs == nil {
		req.ListenChannelIDs = []string{}
	}

	nonce, err := randomNonce()
	if err != nil {
		return nil, err
	}
	now := s.now()
	expires := now.Add(req.TTL)

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Audience:  jwt.ClaimStrings{Audience},
			Subject:   req.Subject,
			ID:        nonce,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expires),
		},
		SessionID:         req.SessionID,
		OwnerID:           req.OwnerID,
		OwnerGeneration:   req.OwnerGeneration,
		Role:              req.Role,
		ProduceChannelIDs: append([]string(nil), req.ProduceChannelIDs...),
		ListenChannelIDs:  append([]string(nil), req.ListenChannelIDs...),
	}

	ticket := &crosstalk.MediaTicket{
		ID:                ulid.Make().String(),
		SessionID:         req.SessionID,
		OwnerID:           req.OwnerID,
		OwnerGeneration:   req.OwnerGeneration,
		Subject:           req.Subject,
		Role:              req.Role,
		ProduceChannelIDs: append([]string(nil), req.ProduceChannelIDs...),
		ListenChannelIDs:  append([]string(nil), req.ListenChannelIDs...),
		ExpiresAt:         expires,
		CreatedAt:         now,
	}
	if err := s.store.Issue(ctx, ticket, nonce); err != nil {
		return nil, err
	}

	token := nonce
	if len(s.signingKey) > 0 {
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := tok.SignedString(s.signingKey)
		if err != nil {
			return nil, fmt.Errorf("sign media ticket: %w", err)
		}
		token = signed
	}

	return &IssuedTicket{
		Token:  token,
		Nonce:  nonce,
		Ticket: *ticket,
		Claims: claims,
	}, nil
}

// ConsumeRequest carries the credential presented by a peer.
type ConsumeRequest struct {
	// Nonce is the plaintext one-time secret (JTI). When Token is a JWT, the
	// nonce is extracted from claims after signature verification.
	Nonce string
	// Token is an optional signed JWT. When set, it is validated first.
	Token string
	// OwnerGeneration is the current fenced generation that must match.
	OwnerGeneration uint64
}

// Consume validates the ticket (optional JWT) then atomically consumes it
// before peer allocation. Auth checks conceptually precede this call; the API
// layer should call Consume only after authorization has passed.
func (s *Service) Consume(ctx context.Context, req ConsumeRequest) (*crosstalk.MediaTicket, error) {
	nonce := req.Nonce
	if req.Token != "" && len(s.signingKey) > 0 {
		claims, err := s.ParseToken(req.Token)
		if err != nil {
			return nil, err
		}
		if claims.ID == "" {
			return nil, fmt.Errorf("%w: missing jti", crosstalk.ErrTicketInvalid)
		}
		nonce = claims.ID
		if claims.OwnerGeneration != req.OwnerGeneration {
			return nil, fmt.Errorf("%w: claim generation %d != %d", crosstalk.ErrStaleGeneration, claims.OwnerGeneration, req.OwnerGeneration)
		}
	}
	if nonce == "" {
		return nil, fmt.Errorf("%w: nonce required", crosstalk.ErrTicketInvalid)
	}
	return s.store.Consume(ctx, nonce, req.OwnerGeneration)
}

// ParseToken validates a signed media JWT and returns its claims.
func (s *Service) ParseToken(tokenStr string) (*Claims, error) {
	if len(s.signingKey) == 0 {
		return nil, fmt.Errorf("%w: no signing key configured", crosstalk.ErrTicketInvalid)
	}
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.signingKey, nil
	}, jwt.WithAudience(Audience))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", crosstalk.ErrTicketInvalid, err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, crosstalk.ErrTicketInvalid
	}
	if claims.ExpiresAt != nil {
		// Cap check: issued lifetime must not exceed MaxTTL.
		if claims.IssuedAt != nil {
			life := claims.ExpiresAt.Sub(claims.IssuedAt.Time)
			if life > MaxTTL+time.Second { // 1s skew tolerance
				return nil, fmt.Errorf("%w: lifetime %s exceeds max", crosstalk.ErrTicketInvalid, life)
			}
		}
	}
	return claims, nil
}

// HashNonce returns the SHA-256 hex digest of a nonce (same as store).
func HashNonce(nonce string) string {
	sum := sha256.Sum256([]byte(nonce))
	return hex.EncodeToString(sum[:])
}

func randomNonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
