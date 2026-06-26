package database

import (
	"database/sql"
	"fmt"
	"strings"
)

func migrate(conn *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS emails (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    message_id TEXT,
    mail_from TEXT,
    rcpt_to TEXT,
    subject TEXT,
    mail_date TEXT,
    text_body TEXT,
    html_body TEXT,
    headers_json TEXT NOT NULL DEFAULT '{}',
    raw_email BLOB NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS attachments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email_id INTEGER NOT NULL REFERENCES emails(id) ON DELETE CASCADE,
    filename TEXT,
    content_type TEXT,
    size INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_emails_created_at ON emails(created_at);
CREATE INDEX IF NOT EXISTS idx_emails_mail_from ON emails(mail_from);
CREATE INDEX IF NOT EXISTS idx_emails_rcpt_to ON emails(rcpt_to);
CREATE INDEX IF NOT EXISTS idx_emails_message_id ON emails(message_id);
`
	if _, err := conn.Exec(schema); err != nil {
		return err
	}
	if err := addColumnIfMissing(conn, "emails", "preview_otk", "TEXT"); err != nil {
		return err
	}
	if err := addColumnIfMissing(conn, "emails", "keep_otk", "TEXT"); err != nil {
		return err
	}
	if _, err := conn.Exec(`UPDATE emails SET keep_otk = preview_otk WHERE keep_otk IS NULL AND preview_otk IS NOT NULL`); err != nil {
		return err
	}
	if _, err := conn.Exec(`UPDATE emails SET keep_otk = lower(hex(randomblob(16))) WHERE keep_otk IS NULL`); err != nil {
		return err
	}
	if err := addColumnIfMissing(conn, "emails", "kept", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	return addColumnIfMissing(conn, "emails", "opened_at", "TEXT")
}

func addColumnIfMissing(conn *sql.DB, table, column, typ string) error {
	_, err := conn.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, typ))
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return nil
	}
	return err
}
