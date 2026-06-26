package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"mewmail/api/internal/auth"
	"mewmail/api/internal/models"
)

// EmailFilter holds list/delete query parameters.
type EmailFilter struct {
	From      string
	To        string
	Subject   string
	MessageID string
	After     *time.Time
	Before    *time.Time
	Kept      *bool
	Opened    *bool
	SinceID   int64
	Limit     int
	Offset    int
}

// EmailStats holds aggregate mailbox counts.
type EmailStats struct {
	Total           int        `json:"total"`
	Kept            int        `json:"kept"`
	Unopened        int        `json:"unopened"`
	OldestCreatedAt *time.Time `json:"oldest_created_at,omitempty"`
}

// InsertEmail stores an email and its attachment metadata in a transaction.
// Returns the new row id, a one-time preview token, and a reusable keep link token.
func (db *DB) InsertEmail(ctx context.Context, e *models.Email) (int64, string, string, error) {
	previewOTK, err := auth.GenerateToken()
	if err != nil {
		return 0, "", "", err
	}
	keepOTK, err := auth.GenerateToken()
	if err != nil {
		return 0, "", "", err
	}

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", "", err
	}
	defer tx.Rollback()

	created := e.CreatedAt.UTC().Format(time.RFC3339)
	res, err := tx.StmtContext(ctx, db.insertEmail).ExecContext(ctx,
		e.MessageID, e.MailFrom, e.RcptTo, e.Subject, e.MailDate,
		e.TextBody, e.HTMLBody, e.HeadersJSON, e.RawEmail, created, previewOTK, keepOTK,
	)
	if err != nil {
		return 0, "", "", fmt.Errorf("insert email: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, "", "", err
	}

	insAttach := tx.StmtContext(ctx, db.insertAttach)
	for _, a := range e.Attachments {
		if _, err := insAttach.ExecContext(ctx, id, a.Filename, a.ContentType, a.Size); err != nil {
			return 0, "", "", fmt.Errorf("insert attachment: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, "", "", err
	}
	return id, previewOTK, keepOTK, nil
}

// ValidPreviewOTK checks a preview token without clearing it.
func (db *DB) ValidPreviewOTK(ctx context.Context, id int64, otk string) (bool, error) {
	if otk == "" {
		return false, nil
	}
	var one int
	err := db.checkPreviewOTK.QueryRowContext(ctx, id, otk).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ConsumePreviewOTK validates a one-time preview token and clears it.
func (db *DB) ConsumePreviewOTK(ctx context.Context, id int64, otk string) (bool, error) {
	if otk == "" {
		return false, nil
	}
	res, err := db.consumePreviewOTK.ExecContext(ctx, id, otk)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ValidKeepOTK checks the keep link token (not consumed on use).
func (db *DB) ValidKeepOTK(ctx context.Context, id int64, otk string) (bool, error) {
	if otk == "" {
		return false, nil
	}
	var one int
	err := db.checkKeepOTK.QueryRowContext(ctx, id, otk).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetLinkOTKs returns preview and keep tokens for link generation. ok is false if missing.
func (db *DB) GetLinkOTKs(ctx context.Context, id int64) (previewOTK, keepOTK string, ok bool, err error) {
	var preview, keep sql.NullString
	err = db.conn.QueryRowContext(ctx, `SELECT preview_otk, keep_otk FROM emails WHERE id = ?`, id).Scan(&preview, &keep)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	if preview.Valid {
		previewOTK = preview.String
	}
	if keep.Valid {
		keepOTK = keep.String
	}
	return previewOTK, keepOTK, true, nil
}

// GetEmail returns a single email by ID.
func (db *DB) GetEmail(ctx context.Context, id int64) (*models.Email, error) {
	row := db.getEmail.QueryRowContext(ctx, id)
	e, err := scanEmailRowFromRow(row)
	if err != nil {
		return nil, err
	}
	atts, err := db.attachmentsFor(ctx, id)
	if err != nil {
		return nil, err
	}
	e.Attachments = atts
	return e, nil
}

// ListEmails returns emails matching filter, newest first.
func (db *DB) ListEmails(ctx context.Context, f EmailFilter) ([]models.Email, int, error) {
	where, args := buildWhere(f)
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	countQ := "SELECT COUNT(*) FROM emails" + where
	var total int
	if err := db.conn.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQ := `SELECT id, message_id, mail_from, rcpt_to, subject, mail_date, text_body, html_body, headers_json, raw_email, created_at, kept, opened_at
		FROM emails` + where + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := db.conn.QueryContext(ctx, listQ, listArgs...)
	if err != nil {
		return nil, 0, err
	}

	var emails []models.Email
	for rows.Next() {
		e, err := scanEmailRow(rows)
		if err != nil {
			rows.Close()
			return nil, 0, err
		}
		e.RawEmail = nil
		emails = append(emails, *e)
	}
	if err := rows.Close(); err != nil {
		return nil, 0, err
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	for i := range emails {
		atts, err := db.attachmentsFor(ctx, emails[i].ID)
		if err != nil {
			return nil, 0, err
		}
		emails[i].Attachments = atts
	}
	if emails == nil {
		emails = []models.Email{}
	}
	return emails, total, nil
}

// EmailExists reports whether an email row exists.
func (db *DB) EmailExists(ctx context.Context, id int64) (bool, error) {
	var one int
	err := db.conn.QueryRowContext(ctx, `SELECT 1 FROM emails WHERE id = ? LIMIT 1`, id).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// EmailStats returns aggregate counts for the mailbox.
func (db *DB) EmailStats(ctx context.Context) (*EmailStats, error) {
	var s EmailStats
	var oldest sql.NullString
	err := db.conn.QueryRowContext(ctx, `
		SELECT COUNT(*),
			COALESCE(SUM(CASE WHEN kept = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN opened_at IS NULL THEN 1 ELSE 0 END), 0),
			MIN(created_at)
		FROM emails`).Scan(&s.Total, &s.Kept, &s.Unopened, &oldest)
	if err != nil {
		return nil, err
	}
	if oldest.Valid {
		t, _ := time.Parse(time.RFC3339, oldest.String)
		s.OldestCreatedAt = &t
	}
	return &s, nil
}

// RegeneratePreviewOTK replaces the preview token for an email. ok is false if missing.
func (db *DB) RegeneratePreviewOTK(ctx context.Context, id int64) (otk string, ok bool, err error) {
	otk, err = auth.GenerateToken()
	if err != nil {
		return "", false, err
	}
	res, err := db.regenOTK.ExecContext(ctx, otk, id)
	if err != nil {
		return "", false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return otk, true, nil
	}
	return "", false, nil
}

// KeepEmail marks an email to survive automatic retention cleanup.
// Status is "kept" on first call or "already_kept" when unchanged. ok is false if missing.
func (db *DB) KeepEmail(ctx context.Context, id int64) (status string, ok bool, err error) {
	res, err := db.keepEmail.ExecContext(ctx, id)
	if err != nil {
		return "", false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return "kept", true, nil
	}
	var kept int
	err = db.conn.QueryRowContext(ctx, `SELECT kept FROM emails WHERE id = ?`, id).Scan(&kept)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if kept != 0 {
		return "already_kept", true, nil
	}
	return "", false, nil
}

// UnkeepEmail clears the retention keep flag.
// Status is "unkept" on first call or "already_unkept" when unchanged. ok is false if missing.
func (db *DB) UnkeepEmail(ctx context.Context, id int64) (status string, ok bool, err error) {
	res, err := db.unkeepEmail.ExecContext(ctx, id)
	if err != nil {
		return "", false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return "unkept", true, nil
	}
	var kept int
	err = db.conn.QueryRowContext(ctx, `SELECT kept FROM emails WHERE id = ?`, id).Scan(&kept)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if kept == 0 {
		return "already_unkept", true, nil
	}
	return "", false, nil
}

// TrackEmailOpen records the first time an email is opened.
// Status is "opened" on first call or "already_opened" when unchanged. ok is false if missing.
func (db *DB) TrackEmailOpen(ctx context.Context, id int64) (status string, ok bool, err error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.trackOpen.ExecContext(ctx, now, id)
	if err != nil {
		return "", false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return "opened", true, nil
	}
	var openedAt sql.NullString
	err = db.conn.QueryRowContext(ctx, `SELECT opened_at FROM emails WHERE id = ?`, id).Scan(&openedAt)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if openedAt.Valid {
		return "already_opened", true, nil
	}
	return "", false, nil
}

// DeleteEmail removes one email by ID.
func (db *DB) DeleteEmail(ctx context.Context, id int64) (bool, error) {
	res, err := db.deleteEmail.ExecContext(ctx, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteEmails removes emails matching filter.
func (db *DB) DeleteEmails(ctx context.Context, f EmailFilter) (int64, error) {
	where, args := buildWhere(f)
	q := "DELETE FROM emails" + where
	res, err := db.conn.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (db *DB) attachmentsFor(ctx context.Context, emailID int64) ([]models.Attachment, error) {
	rows, err := db.listAttach.QueryContext(ctx, emailID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var atts []models.Attachment
	for rows.Next() {
		var a models.Attachment
		if err := rows.Scan(&a.ID, &a.EmailID, &a.Filename, &a.ContentType, &a.Size); err != nil {
			return nil, err
		}
		atts = append(atts, a)
	}
	if atts == nil {
		atts = []models.Attachment{}
	}
	return atts, rows.Err()
}

func buildWhere(f EmailFilter) (string, []any) {
	var clauses []string
	var args []any
	if f.From != "" {
		clauses = append(clauses, "mail_from LIKE ?")
		args = append(args, "%"+f.From+"%")
	}
	if f.To != "" {
		clauses = append(clauses, "rcpt_to LIKE ?")
		args = append(args, "%"+f.To+"%")
	}
	if f.Subject != "" {
		clauses = append(clauses, "subject LIKE ?")
		args = append(args, "%"+f.Subject+"%")
	}
	if f.MessageID != "" {
		clauses = append(clauses, "message_id = ?")
		args = append(args, f.MessageID)
	}
	if f.SinceID > 0 {
		clauses = append(clauses, "id > ?")
		args = append(args, f.SinceID)
	}
	if f.After != nil {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, f.After.UTC().Format(time.RFC3339))
	}
	if f.Before != nil {
		clauses = append(clauses, "created_at <= ?")
		args = append(args, f.Before.UTC().Format(time.RFC3339))
	}
	if f.Kept != nil {
		clauses = append(clauses, "kept = ?")
		if *f.Kept {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	if f.Opened != nil {
		if *f.Opened {
			clauses = append(clauses, "opened_at IS NOT NULL")
		} else {
			clauses = append(clauses, "opened_at IS NULL")
		}
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func scanEmailFields(e *models.Email, raw []byte, created string, kept int, openedAt sql.NullString) {
	e.RawEmail = raw
	e.CreatedAt, _ = time.Parse(time.RFC3339, created)
	e.Kept = kept != 0
	if openedAt.Valid {
		t, _ := time.Parse(time.RFC3339, openedAt.String)
		e.OpenedAt = &t
	}
}

func scanEmailRowFromRow(row *sql.Row) (*models.Email, error) {
	var e models.Email
	var created string
	var raw []byte
	var kept int
	var openedAt sql.NullString
	err := row.Scan(
		&e.ID, &e.MessageID, &e.MailFrom, &e.RcptTo, &e.Subject, &e.MailDate,
		&e.TextBody, &e.HTMLBody, &e.HeadersJSON, &raw, &created, &kept, &openedAt,
	)
	if err != nil {
		return nil, err
	}
	scanEmailFields(&e, raw, created, kept, openedAt)
	return &e, nil
}

func scanEmailRow(rows *sql.Rows) (*models.Email, error) {
	var e models.Email
	var created string
	var raw []byte
	var kept int
	var openedAt sql.NullString
	err := rows.Scan(
		&e.ID, &e.MessageID, &e.MailFrom, &e.RcptTo, &e.Subject, &e.MailDate,
		&e.TextBody, &e.HTMLBody, &e.HeadersJSON, &raw, &created, &kept, &openedAt,
	)
	if err != nil {
		return nil, err
	}
	scanEmailFields(&e, raw, created, kept, openedAt)
	return &e, nil
}
