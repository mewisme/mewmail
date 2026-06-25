package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite connection and prepared statements.
type DB struct {
	conn *sql.DB

	insertEmail    *sql.Stmt
	insertAttach   *sql.Stmt
	getEmail       *sql.Stmt
	listEmails     *sql.Stmt
	countEmails    *sql.Stmt
	deleteEmail    *sql.Stmt
	deleteExpired  *sql.Stmt
	keepEmail      *sql.Stmt
	checkOTK       *sql.Stmt
	consumeOTK     *sql.Stmt
	listAttach     *sql.Stmt
	deleteFiltered *sql.Stmt
	countFiltered  *sql.Stmt
}

// Open opens the database, applies PRAGMAs, and prepares statements.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA cache_size=-2000",
	} {
		if _, err := conn.Exec(pragma); err != nil {
			conn.Close()
			return nil, fmt.Errorf("pragma %q: %w", pragma, err)
		}
	}

	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, err
	}

	db := &DB{conn: conn}
	if err := db.prepare(); err != nil {
		conn.Close()
		return nil, err
	}
	return db, nil
}

func (db *DB) prepare() error {
	var err error
	stmts := []struct {
		dest **sql.Stmt
		q    string
	}{
		{&db.insertEmail, `INSERT INTO emails (message_id, mail_from, rcpt_to, subject, mail_date, text_body, html_body, headers_json, raw_email, created_at, preview_otk)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`},
		{&db.insertAttach, `INSERT INTO attachments (email_id, filename, content_type, size) VALUES (?, ?, ?, ?)`},
		{&db.getEmail, `SELECT id, message_id, mail_from, rcpt_to, subject, mail_date, text_body, html_body, headers_json, raw_email, created_at, kept FROM emails WHERE id = ?`},
		{&db.listAttach, `SELECT id, email_id, filename, content_type, size FROM attachments WHERE email_id = ?`},
		{&db.deleteEmail, `DELETE FROM emails WHERE id = ?`},
		{&db.deleteExpired, `DELETE FROM emails WHERE created_at < ? AND kept = 0`},
		{&db.keepEmail, `UPDATE emails SET kept = 1 WHERE id = ? AND kept = 0`},
		{&db.checkOTK, `SELECT 1 FROM emails WHERE id = ? AND preview_otk = ? LIMIT 1`},
		{&db.consumeOTK, `UPDATE emails SET preview_otk = NULL WHERE id = ? AND preview_otk = ?`},
	}

	for _, s := range stmts {
		*s.dest, err = db.conn.Prepare(s.q)
		if err != nil {
			return fmt.Errorf("prepare: %w", err)
		}
	}
	return nil
}

// Close closes the database and prepared statements.
func (db *DB) Close() error {
	for _, s := range []*sql.Stmt{
		db.insertEmail, db.insertAttach, db.getEmail, db.listAttach,
		db.deleteEmail, db.deleteExpired, db.keepEmail, db.checkOTK, db.consumeOTK,
		db.listEmails, db.countEmails, db.deleteFiltered, db.countFiltered,
	} {
		if s != nil {
			_ = s.Close()
		}
	}
	return db.conn.Close()
}

// Vacuum reclaims disk space.
func (db *DB) Vacuum() error {
	_, err := db.conn.Exec("VACUUM")
	return err
}

// Conn returns the underlying connection for tests.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// DeleteExpired removes emails older than cutoff.
func (db *DB) DeleteExpired(cutoff time.Time) (int64, error) {
	res, err := db.deleteExpired.Exec(cutoff.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
