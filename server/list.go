package crosstalk

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Collection page bounds for top-level SPA lists.
const (
	DefaultListLimit = 25
	MaxListLimit     = 100
)

// ErrInvalidListQuery is returned for bad sort/direction/cursor/limit values.
var ErrInvalidListQuery = errors.New("invalid list query")

// ListDirection is the sort direction for collection queries.
type ListDirection string

const (
	ListAsc  ListDirection = "asc"
	ListDesc ListDirection = "desc"
)

// ListQuery is the shared typed query contract for top-level collections.
// RestrictToIDs, when non-nil, scopes results to that ID set before pagination
// (used for translator assignment isolation). A non-nil empty slice yields an
// empty page without scanning the full table.
type ListQuery struct {
	Q            string
	Sort         string
	Direction    ListDirection
	Limit        int
	Cursor       string
	RestrictToIDs *[]string
}

// SessionPage is a bounded page of sessions.
type SessionPage struct {
	Items      []Session
	NextCursor string
	Total      *int64
}

// ABCListItem is an ABC row with optional batch-resolved session name.
type ABCListItem struct {
	ABC
	SessionName string
}

// ABCPage is a bounded page of ABCs.
type ABCPage struct {
	Items      []ABCListItem
	NextCursor string
	Total      *int64
}

// TranslatorListItem is a translator with assigned sessions and names.
type TranslatorListItem struct {
	User
	SessionIDs   []string
	SessionNames map[string]string // session_id → name (bounded lookup)
}

// TranslatorPage is a bounded page of translators.
type TranslatorPage struct {
	Items      []TranslatorListItem
	NextCursor string
	Total      *int64
}

// BoundListLimit clamps limit to [1, MaxListLimit], defaulting empty/zero to DefaultListLimit.
func BoundListLimit(limit int) int {
	if limit <= 0 {
		return DefaultListLimit
	}
	if limit > MaxListLimit {
		return MaxListLimit
	}
	return limit
}

// NormalizeListDirection returns asc/desc or an error.
func NormalizeListDirection(dir string) (ListDirection, error) {
	switch strings.ToLower(strings.TrimSpace(dir)) {
	case "", string(ListDesc):
		return ListDesc, nil
	case string(ListAsc):
		return ListAsc, nil
	default:
		return "", fmt.Errorf("%w: direction %q", ErrInvalidListQuery, dir)
	}
}

// listCursor is the opaque keyset payload.
type listCursor struct {
	V    int       `json:"v"`
	Sort string    `json:"s"`
	Dir  string    `json:"d"`
	Q    string    `json:"q,omitempty"`
	TS   time.Time `json:"t,omitempty"`
	Name string    `json:"n,omitempty"`
	ID   string    `json:"i"`
}

// EncodeListCursor builds an opaque cursor from the last returned row keys.
func EncodeListCursor(sort string, dir ListDirection, q, id, name string, ts time.Time) string {
	payload := listCursor{
		V:    1,
		Sort: sort,
		Dir:  string(dir),
		Q:    q,
		TS:   ts.UTC(),
		Name: name,
		ID:   id,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// DecodeListCursor parses and validates an opaque cursor against the active query.
func DecodeListCursor(token, sort string, dir ListDirection, q string) (id, name string, ts time.Time, err error) {
	if token == "" {
		return "", "", time.Time{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("%w: cursor", ErrInvalidListQuery)
	}
	var c listCursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", "", time.Time{}, fmt.Errorf("%w: cursor", ErrInvalidListQuery)
	}
	if c.V != 1 || c.ID == "" {
		return "", "", time.Time{}, fmt.Errorf("%w: cursor", ErrInvalidListQuery)
	}
	if c.Sort != sort || c.Dir != string(dir) || c.Q != q {
		return "", "", time.Time{}, fmt.Errorf("%w: cursor mismatch", ErrInvalidListQuery)
	}
	return c.ID, c.Name, c.TS.UTC(), nil
}
