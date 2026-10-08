package app

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

// ViewThread is a durable discussion thread anchored to a view, element, or
// connector.
type ViewThread struct {
	ID                int32
	WorkspaceID       string
	ViewID            int32
	ElementID         *int32
	ConnectorID       *int32
	CreatedBy         string
	CreatedByUsername string
	Status            string
	CreatedAt         string
	ResolvedAt        *string
}

// ViewComment is a comment posted within a ViewThread.
type ViewComment struct {
	ID             int32
	WorkspaceID    string
	ViewID         int32
	ThreadID       int32
	AuthorID       string
	AuthorUsername string
	Body           string
	CreatedAt      string
	UpdatedAt      string
}

// ElementReactionSummary aggregates emoji reactions on an element for a view.
type ElementReactionSummary struct {
	ElementID   int32
	Emoji       string
	Count       int32
	ReactedByMe bool
}

// Drawing is durable freehand/text drawing state for a view.
type Drawing struct {
	PathID   string
	UserID   string
	Points   []byte
	Color    string
	Width    float64
	Text     string
	FontSize float64
}

// workspaceKey is the workspace_id column value used by collaboration rows.
// The nil workspace maps to the local sentinel.
func workspaceKey(workspaceID uuid.UUID) string {
	if workspaceID == uuid.Nil {
		return "local"
	}
	return workspaceID.String()
}

func (s *Store) ListViewThreads(ctx context.Context, workspaceID uuid.UUID, viewID int32, elementID, connectorID *int32) ([]ViewThread, error) {
	where := `workspace_id = ? AND view_id = ? AND element_id = ?`
	targetID := any(elementID)
	if connectorID != nil {
		where = `workspace_id = ? AND view_id = ? AND connector_id = ?`
		targetID = connectorID
	}
	var rows []struct {
		ID                int32   `bun:"id"`
		WorkspaceID       string  `bun:"workspace_id"`
		ViewID            int32   `bun:"view_id"`
		ElementID         *int32  `bun:"element_id"`
		ConnectorID       *int32  `bun:"connector_id"`
		CreatedBy         string  `bun:"created_by"`
		CreatedByUsername string  `bun:"created_by_username"`
		Status            string  `bun:"status"`
		CreatedAt         string  `bun:"created_at"`
		ResolvedAt        *string `bun:"resolved_at"`
	}
	err := s.QueryDB().NewRaw(
		`SELECT t.id, t.workspace_id, t.view_id, t.element_id, t.connector_id, t.created_by,
		        COALESCE(NULLIF(t.created_by_username, ''), t.created_by) AS created_by_username,
		        t.status, t.created_at, t.resolved_at
		 FROM view_threads t
		 WHERE `+where+`
		 ORDER BY t.created_at ASC`,
		workspaceKey(workspaceID), viewID, targetID,
	).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	out := make([]ViewThread, 0, len(rows))
	for _, row := range rows {
		out = append(out, ViewThread{
			ID:                row.ID,
			WorkspaceID:       row.WorkspaceID,
			ViewID:            row.ViewID,
			ElementID:         row.ElementID,
			ConnectorID:       row.ConnectorID,
			CreatedBy:         row.CreatedBy,
			CreatedByUsername: row.CreatedByUsername,
			Status:            row.Status,
			CreatedAt:         row.CreatedAt,
			ResolvedAt:        row.ResolvedAt,
		})
	}
	return out, nil
}

func (s *Store) CreateViewThread(ctx context.Context, workspaceID uuid.UUID, viewID int32, elementID, connectorID *int32, createdBy, createdByUsername string) (int32, error) {
	var row struct {
		ID int32 `bun:"id"`
	}
	err := s.QueryDB().NewRaw(
		`INSERT INTO view_threads (workspace_id, view_id, element_id, connector_id, created_by, created_by_username, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'open', ?)
		 RETURNING id`,
		workspaceKey(workspaceID), viewID, elementID, connectorID, createdBy, createdByUsername, NowString(),
	).Scan(ctx, &row)
	if err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (s *Store) ViewThreadByID(ctx context.Context, workspaceID uuid.UUID, viewID, threadID int32) (ViewThread, error) {
	var row struct {
		ID                int32   `bun:"id"`
		WorkspaceID       string  `bun:"workspace_id"`
		ViewID            int32   `bun:"view_id"`
		ElementID         *int32  `bun:"element_id"`
		ConnectorID       *int32  `bun:"connector_id"`
		CreatedBy         string  `bun:"created_by"`
		CreatedByUsername string  `bun:"created_by_username"`
		Status            string  `bun:"status"`
		CreatedAt         string  `bun:"created_at"`
		ResolvedAt        *string `bun:"resolved_at"`
	}
	err := s.QueryDB().NewRaw(
		`SELECT t.id, t.workspace_id, t.view_id, t.element_id, t.connector_id, t.created_by,
		        COALESCE(NULLIF(t.created_by_username, ''), t.created_by) AS created_by_username,
		        t.status, t.created_at, t.resolved_at
		 FROM view_threads t
		 WHERE t.workspace_id = ? AND t.view_id = ? AND t.id = ?`,
		workspaceKey(workspaceID), viewID, threadID,
	).Scan(ctx, &row)
	if err != nil {
		return ViewThread{}, err
	}
	return ViewThread{
		ID:                row.ID,
		WorkspaceID:       row.WorkspaceID,
		ViewID:            row.ViewID,
		ElementID:         row.ElementID,
		ConnectorID:       row.ConnectorID,
		CreatedBy:         row.CreatedBy,
		CreatedByUsername: row.CreatedByUsername,
		Status:            row.Status,
		CreatedAt:         row.CreatedAt,
		ResolvedAt:        row.ResolvedAt,
	}, nil
}

func (s *Store) SetViewThreadResolved(ctx context.Context, workspaceID uuid.UUID, viewID, threadID int32, resolved bool) error {
	status := "open"
	var resolvedAt *string
	if resolved {
		status = "resolved"
		now := NowString()
		resolvedAt = &now
	}
	_, err := s.QueryDB().NewRaw(
		`UPDATE view_threads
		 SET status = ?, resolved_at = ?
		 WHERE workspace_id = ? AND view_id = ? AND id = ?`,
		status, resolvedAt, workspaceKey(workspaceID), viewID, threadID,
	).Exec(ctx)
	return err
}

func (s *Store) CreateViewComment(ctx context.Context, workspaceID uuid.UUID, viewID, threadID int32, authorID, authorUsername, body string) (ViewComment, error) {
	now := NowString()
	var row struct {
		ID int32 `bun:"id"`
	}
	err := s.QueryDB().NewRaw(
		`INSERT INTO view_comments (workspace_id, view_id, thread_id, author_id, author_username, body, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 RETURNING id`,
		workspaceKey(workspaceID), viewID, threadID, authorID, authorUsername, body, now, now,
	).Scan(ctx, &row)
	if err != nil {
		return ViewComment{}, err
	}
	comments, err := s.ListViewComments(ctx, workspaceID, viewID, threadID)
	if err != nil {
		return ViewComment{}, err
	}
	for _, comment := range comments {
		if comment.ID == row.ID {
			return comment, nil
		}
	}
	return ViewComment{}, sql.ErrNoRows
}

func (s *Store) ListViewComments(ctx context.Context, workspaceID uuid.UUID, viewID, threadID int32) ([]ViewComment, error) {
	var rows []struct {
		ID             int32  `bun:"id"`
		WorkspaceID    string `bun:"workspace_id"`
		ViewID         int32  `bun:"view_id"`
		ThreadID       int32  `bun:"thread_id"`
		AuthorID       string `bun:"author_id"`
		AuthorUsername string `bun:"author_username"`
		Body           string `bun:"body"`
		CreatedAt      string `bun:"created_at"`
		UpdatedAt      string `bun:"updated_at"`
	}
	err := s.QueryDB().NewRaw(
		`SELECT c.id, c.workspace_id, c.view_id, c.thread_id, c.author_id,
		        COALESCE(NULLIF(c.author_username, ''), c.author_id) AS author_username,
		        c.body, c.created_at, c.updated_at
		 FROM view_comments c
		 WHERE c.workspace_id = ? AND c.view_id = ? AND c.thread_id = ?
		 ORDER BY c.created_at ASC`,
		workspaceKey(workspaceID), viewID, threadID,
	).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	out := make([]ViewComment, 0, len(rows))
	for _, row := range rows {
		out = append(out, ViewComment{
			ID:             row.ID,
			WorkspaceID:    row.WorkspaceID,
			ViewID:         row.ViewID,
			ThreadID:       row.ThreadID,
			AuthorID:       row.AuthorID,
			AuthorUsername: row.AuthorUsername,
			Body:           row.Body,
			CreatedAt:      row.CreatedAt,
			UpdatedAt:      row.UpdatedAt,
		})
	}
	return out, nil
}

func (s *Store) ListElementReactions(ctx context.Context, workspaceID uuid.UUID, viewID int32, userID string) ([]ElementReactionSummary, error) {
	var rows []struct {
		ElementID   int32  `bun:"element_id"`
		Emoji       string `bun:"emoji"`
		Count       int32  `bun:"reaction_count"`
		ReactedByMe int32  `bun:"reacted_by_me"`
	}
	err := s.QueryDB().NewRaw(
		`SELECT element_id, emoji, COUNT(*) AS reaction_count,
		        MAX(CASE WHEN user_id = ? THEN 1 ELSE 0 END) AS reacted_by_me
		 FROM element_reactions
		 WHERE workspace_id = ? AND view_id = ?
		 GROUP BY element_id, emoji
		 ORDER BY element_id ASC, emoji ASC`,
		userID, workspaceKey(workspaceID), viewID,
	).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	out := make([]ElementReactionSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, ElementReactionSummary{
			ElementID:   row.ElementID,
			Emoji:       row.Emoji,
			Count:       row.Count,
			ReactedByMe: row.ReactedByMe > 0,
		})
	}
	return out, nil
}

// ToggleElementReaction adds the reaction if absent and removes it if present.
// It reports whether the reaction now exists.
func (s *Store) ToggleElementReaction(ctx context.Context, workspaceID uuid.UUID, viewID, elementID int32, userID, emoji string) (bool, error) {
	result, err := s.QueryDB().NewRaw(
		`DELETE FROM element_reactions
		 WHERE workspace_id = ? AND view_id = ? AND element_id = ? AND user_id = ? AND emoji = ?`,
		workspaceKey(workspaceID), viewID, elementID, userID, emoji,
	).Exec(ctx)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		return false, nil
	}
	_, err = s.QueryDB().NewRaw(
		`INSERT INTO element_reactions (workspace_id, view_id, element_id, user_id, emoji, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		workspaceKey(workspaceID), viewID, elementID, userID, emoji, NowString(),
	).Exec(ctx)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ListDrawings(ctx context.Context, workspaceID uuid.UUID, viewID int32) ([]Drawing, error) {
	var rows []struct {
		PathID   string  `bun:"path_id"`
		UserID   string  `bun:"user_id"`
		Points   string  `bun:"points"`
		Color    string  `bun:"color"`
		Width    float64 `bun:"width"`
		Text     string  `bun:"text"`
		FontSize float64 `bun:"font_size"`
	}
	err := s.QueryDB().NewRaw(
		`SELECT path_id, user_id, points, color, width, text, font_size
		 FROM drawings
		 WHERE workspace_id = ? AND view_id = ?
		 ORDER BY created_at ASC`,
		workspaceKey(workspaceID), viewID,
	).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	out := make([]Drawing, 0, len(rows))
	for _, row := range rows {
		out = append(out, Drawing{
			PathID:   row.PathID,
			UserID:   row.UserID,
			Points:   []byte(row.Points),
			Color:    row.Color,
			Width:    row.Width,
			Text:     row.Text,
			FontSize: row.FontSize,
		})
	}
	return out, nil
}

func (s *Store) UpsertDrawing(ctx context.Context, workspaceID uuid.UUID, viewID int32, d Drawing) error {
	now := NowString()
	_, err := s.QueryDB().NewRaw(
		`INSERT INTO drawings (workspace_id, view_id, user_id, path_id, points, color, width, text, font_size, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (workspace_id, view_id, path_id) DO UPDATE SET
		   user_id = excluded.user_id,
		   points = excluded.points,
		   color = excluded.color,
		   width = excluded.width,
		   text = excluded.text,
		   font_size = excluded.font_size,
		   updated_at = excluded.updated_at`,
		workspaceKey(workspaceID), viewID, d.UserID, d.PathID, string(d.Points), d.Color, d.Width, d.Text, d.FontSize, now, now,
	).Exec(ctx)
	return err
}

func (s *Store) DeleteDrawing(ctx context.Context, workspaceID uuid.UUID, viewID int32, pathID string) error {
	_, err := s.QueryDB().NewRaw(
		`DELETE FROM drawings WHERE workspace_id = ? AND view_id = ? AND path_id = ?`,
		workspaceKey(workspaceID), viewID, pathID,
	).Exec(ctx)
	return err
}
