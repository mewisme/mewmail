package database

import "database/sql"

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
	_, err := conn.Exec(schema)
	return err
}
