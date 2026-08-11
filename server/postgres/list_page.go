package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

var sessionSortColumns = map[string]string{
	"":           "created_at",
	"created_at": "created_at",
	"updated_at": "updated_at",
	"name":       "name",
	"id":         "id",
}

var abcSortColumns = map[string]string{
	"":           "created_at",
	"created_at": "created_at",
	"name":       "name",
	"id":         "id",
}

var translatorSortColumns = map[string]string{
	"":           "created_at",
	"created_at": "created_at",
	"username":   "username",
	"id":         "id",
}

func resolveSort(allow map[string]string, sort string) (string, error) {
	col, ok := allow[strings.ToLower(strings.TrimSpace(sort))]
	if !ok {
		return "", fmt.Errorf("%w: sort %q", crosstalk.ErrInvalidListQuery, sort)
	}
	return col, nil
}

func normalizeQuery(q crosstalk.ListQuery) (crosstalk.ListQuery, crosstalk.ListDirection, string, error) {
	dir, err := crosstalk.NormalizeListDirection(string(q.Direction))
	if err != nil {
		return q, "", "", err
	}
	q.Direction = dir
	q.Limit = crosstalk.BoundListLimit(q.Limit)
	q.Q = strings.TrimSpace(q.Q)
	return q, dir, q.Q, nil
}
func (s *SessionStore) ListPage(ctx context.Context, q crosstalk.ListQuery) (crosstalk.SessionPage, error) {
	q, dir, search, err := normalizeQuery(q)
	if err != nil {
		return crosstalk.SessionPage{}, err
	}
	sortCol, err := resolveSort(sessionSortColumns, q.Sort)
	if err != nil {
		return crosstalk.SessionPage{}, err
	}
	// Canonical sort key for cursor binding.
	sortKey := sortCol

	if q.RestrictToIDs != nil && len(*q.RestrictToIDs) == 0 {
		zero := int64(0)
		return crosstalk.SessionPage{Items: []crosstalk.Session{}, Total: &zero}, nil
	}

	base := s.db.NewSelect().Model((*sessionModel)(nil))
	if q.RestrictToIDs != nil {
		base = base.Where("id IN (?)", bun.In(*q.RestrictToIDs))
	}
	if search != "" {
		like := "%" + escapeLike(search) + "%"
		base = base.Where("name ILIKE ? ESCAPE '\\'", like)
	}

	total, err := base.Count(ctx)
	if err != nil {
		return crosstalk.SessionPage{}, err
	}
	total64 := int64(total)

	id, name, ts, err := crosstalk.DecodeListCursor(q.Cursor, sortKey, dir, search)
	if err != nil {
		return crosstalk.SessionPage{}, err
	}

	sel := base.ColumnExpr("sess.*")
	if id != "" {
		sel = applyKeyset(sel, "sess", sortCol, dir, id, name, ts)
	}
	orderSQL := orderClause("sess", sortCol, dir)
	var models []sessionModel
	err = sel.OrderExpr(orderSQL).Limit(q.Limit + 1).Scan(ctx, &models)
	if err != nil {
		return crosstalk.SessionPage{}, err
	}

	page := crosstalk.SessionPage{Items: make([]crosstalk.Session, 0, len(models)), Total: &total64}
	hasMore := len(models) > q.Limit
	if hasMore {
		models = models[:q.Limit]
	}
	for i := range models {
		page.Items = append(page.Items, models[i].toDomain())
	}
	if hasMore && len(page.Items) > 0 {
		last := page.Items[len(page.Items)-1]
		page.NextCursor = crosstalk.EncodeListCursor(sortKey, dir, search, last.ID, last.Name, cursorTime(sortCol, last.CreatedAt, last.UpdatedAt))
	}
	return page, nil
}

func (s *ABCStore) ListPage(ctx context.Context, q crosstalk.ListQuery) (crosstalk.ABCPage, error) {
	q, dir, search, err := normalizeQuery(q)
	if err != nil {
		return crosstalk.ABCPage{}, err
	}
	sortCol, err := resolveSort(abcSortColumns, q.Sort)
	if err != nil {
		return crosstalk.ABCPage{}, err
	}
	sortKey := sortCol

	if q.RestrictToIDs != nil && len(*q.RestrictToIDs) == 0 {
		zero := int64(0)
		return crosstalk.ABCPage{Items: []crosstalk.ABCListItem{}, Total: &zero}, nil
	}

	base := s.db.NewSelect().Model((*abcModel)(nil))
	if q.RestrictToIDs != nil {
		// Scope by assigned session_id (not ABC id).
		base = base.Where("session_id IN (?)", bun.In(*q.RestrictToIDs))
	}
	if search != "" {
		like := "%" + escapeLike(search) + "%"
		base = base.Where("name ILIKE ? ESCAPE '\\'", like)
	}

	total, err := base.Count(ctx)
	if err != nil {
		return crosstalk.ABCPage{}, err
	}
	total64 := int64(total)

	id, name, ts, err := crosstalk.DecodeListCursor(q.Cursor, sortKey, dir, search)
	if err != nil {
		return crosstalk.ABCPage{}, err
	}

	type abcRow struct {
		abcModel
		SessionName string `bun:"session_name"`
	}

	sel := s.db.NewSelect().
		ColumnExpr("abc.*").
		ColumnExpr("COALESCE(sess.name, '') AS session_name").
		TableExpr("abcs AS abc").
		Join("LEFT JOIN sessions AS sess ON sess.id = abc.session_id")
	if q.RestrictToIDs != nil {
		sel = sel.Where("abc.session_id IN (?)", bun.In(*q.RestrictToIDs))
	}
	if search != "" {
		like := "%" + escapeLike(search) + "%"
		sel = sel.Where("abc.name ILIKE ? ESCAPE '\\'", like)
	}
	if id != "" {
		sel = applyKeyset(sel, "abc", sortCol, dir, id, name, ts)
	}
	var rows []abcRow
	err = sel.OrderExpr(orderClause("abc", sortCol, dir)).Limit(q.Limit + 1).Scan(ctx, &rows)
	if err != nil {
		return crosstalk.ABCPage{}, err
	}

	page := crosstalk.ABCPage{Items: make([]crosstalk.ABCListItem, 0, len(rows)), Total: &total64}
	hasMore := len(rows) > q.Limit
	if hasMore {
		rows = rows[:q.Limit]
	}
	for i := range rows {
		page.Items = append(page.Items, crosstalk.ABCListItem{
			ABC:         rows[i].toDomain(),
			SessionName: rows[i].SessionName,
		})
	}
	if hasMore && len(page.Items) > 0 {
		last := page.Items[len(page.Items)-1]
		page.NextCursor = crosstalk.EncodeListCursor(sortKey, dir, search, last.ID, last.Name, last.CreatedAt)
	}
	return page, nil
}

func (s *UserStore) ListTranslatorsPage(ctx context.Context, q crosstalk.ListQuery) (crosstalk.TranslatorPage, error) {
	q, dir, search, err := normalizeQuery(q)
	if err != nil {
		return crosstalk.TranslatorPage{}, err
	}
	sortCol, err := resolveSort(translatorSortColumns, q.Sort)
	if err != nil {
		return crosstalk.TranslatorPage{}, err
	}
	sortKey := sortCol

	base := s.db.NewSelect().Model((*userModel)(nil)).Where("role = ?", "translator")
	if search != "" {
		like := "%" + escapeLike(search) + "%"
		base = base.Where("username ILIKE ? ESCAPE '\\'", like)
	}

	total, err := base.Count(ctx)
	if err != nil {
		return crosstalk.TranslatorPage{}, err
	}
	total64 := int64(total)

	id, name, ts, err := crosstalk.DecodeListCursor(q.Cursor, sortKey, dir, search)
	if err != nil {
		return crosstalk.TranslatorPage{}, err
	}

	sel := base.ColumnExpr("usr.*")
	if id != "" {
		sel = applyKeyset(sel, "usr", sortCol, dir, id, name, ts)
	}
	var models []userModel
	err = sel.OrderExpr(orderClause("usr", sortCol, dir)).Limit(q.Limit + 1).Scan(ctx, &models)
	if err != nil {
		return crosstalk.TranslatorPage{}, err
	}

	hasMore := len(models) > q.Limit
	if hasMore {
		models = models[:q.Limit]
	}

	users := usersToDomain(models)
	// Batch-load assignments for this page only.
	trIDs := make([]string, 0, len(users))
	for _, u := range users {
		trIDs = append(trIDs, u.ID)
	}
	assignMap := map[string][]string{}
	sessionIDSet := map[string]struct{}{}
	if len(trIDs) > 0 {
		var links []translatorSessionModel
		err = s.db.NewSelect().Model(&links).
			Where("translator_id IN (?)", bun.In(trIDs)).
			Scan(ctx)
		if err != nil {
			return crosstalk.TranslatorPage{}, err
		}
		for _, l := range links {
			assignMap[l.TranslatorID] = append(assignMap[l.TranslatorID], l.SessionID)
			sessionIDSet[l.SessionID] = struct{}{}
		}
	}
	// One bounded session name lookup for all page assignments.
	sessionNames := map[string]string{}
	if len(sessionIDSet) > 0 {
		ids := make([]string, 0, len(sessionIDSet))
		for id := range sessionIDSet {
			ids = append(ids, id)
		}
		var sess []sessionModel
		err = s.db.NewSelect().Model(&sess).
			Column("id", "name").
			Where("id IN (?)", bun.In(ids)).
			Scan(ctx)
		if err != nil {
			return crosstalk.TranslatorPage{}, err
		}
		for _, sm := range sess {
			sessionNames[sm.ID] = sm.Name
		}
	}

	page := crosstalk.TranslatorPage{
		Items: make([]crosstalk.TranslatorListItem, 0, len(users)),
		Total: &total64,
	}
	for _, u := range users {
		sids := assignMap[u.ID]
		if sids == nil {
			sids = []string{}
		}
		names := map[string]string{}
		for _, sid := range sids {
			if n, ok := sessionNames[sid]; ok {
				names[sid] = n
			}
		}
		page.Items = append(page.Items, crosstalk.TranslatorListItem{
			User:         u,
			SessionIDs:   sids,
			SessionNames: names,
		})
	}
	if hasMore && len(page.Items) > 0 {
		last := page.Items[len(page.Items)-1]
		page.NextCursor = crosstalk.EncodeListCursor(sortKey, dir, search, last.ID, last.Username, last.CreatedAt)
	}
	return page, nil
}

func applyKeyset(q *bun.SelectQuery, alias, sortCol string, dir crosstalk.ListDirection, id, name string, ts time.Time) *bun.SelectQuery {
	col := alias + "." + sortCol
	idCol := alias + ".id"
	op := "<"
	if dir == crosstalk.ListAsc {
		op = ">"
	}
	switch sortCol {
	case "name", "username":
		// (name, id) composite
		return q.Where(fmt.Sprintf("(%s, %s) %s (?, ?)", col, idCol, op), name, id)
	case "created_at", "updated_at":
		return q.Where(fmt.Sprintf("(%s, %s) %s (?, ?)", col, idCol, op), ts, id)
	default: // id
		return q.Where(fmt.Sprintf("%s %s ?", idCol, op), id)
	}
}

func orderClause(alias, sortCol string, dir crosstalk.ListDirection) string {
	d := "DESC"
	if dir == crosstalk.ListAsc {
		d = "ASC"
	}
	if sortCol == "id" {
		return fmt.Sprintf("%s.id %s", alias, d)
	}
	return fmt.Sprintf("%s.%s %s, %s.id %s", alias, sortCol, d, alias, d)
}

func cursorTime(sortCol string, created, updated time.Time) time.Time {
	if sortCol == "updated_at" {
		return updated
	}
	return created
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
