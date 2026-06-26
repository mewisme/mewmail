package cleaner_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"mewmail/api/internal/cleaner"
	"mewmail/api/internal/database"
	"mewmail/api/internal/models"
)

func TestCleaner_DeletesExpired(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/test.db"
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	old := &models.Email{
		MailFrom:    "old@example.com",
		RcptTo:      "a@example.com",
		Subject:     "old",
		HeadersJSON: "{}",
		RawEmail:    []byte("raw"),
		CreatedAt:   time.Now().UTC().Add(-250 * time.Hour),
		Attachments: []models.Attachment{{Filename: "f.txt", ContentType: "text/plain", Size: 1}},
	}
	newer := &models.Email{
		MailFrom:    "new@example.com",
		RcptTo:      "b@example.com",
		Subject:     "new",
		HeadersJSON: "{}",
		RawEmail:    []byte("raw"),
		CreatedAt:   time.Now().UTC(),
	}

	if _, _, _, err := db.InsertEmail(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := db.InsertEmail(context.Background(), newer); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	cl := cleaner.New(db, log, 168, nil)
	n, err := cl.CleanOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted %d, want 1", n)
	}

	emails, total, err := db.ListEmails(context.Background(), database.EmailFilter{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(emails) != 1 {
		t.Fatalf("remaining %d", total)
	}
	if emails[0].MailFrom != "new@example.com" {
		t.Fatalf("wrong email kept: %s", emails[0].MailFrom)
	}
}

func TestCleaner_SkipsKeptEmail(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	old := &models.Email{
		MailFrom:    "old@example.com",
		RcptTo:      "a@example.com",
		Subject:     "old",
		HeadersJSON: "{}",
		RawEmail:    []byte("raw"),
		CreatedAt:   time.Now().UTC().Add(-250 * time.Hour),
	}
	id, _, _, err := db.InsertEmail(context.Background(), old)
	if err != nil {
		t.Fatal(err)
	}
	if status, ok, err := db.KeepEmail(context.Background(), id); err != nil || !ok || status != "kept" {
		t.Fatalf("keep email: status=%q ok=%v err=%v", status, ok, err)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	cl := cleaner.New(db, log, 168, nil)
	n, err := cl.CleanOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("deleted %d, want 0", n)
	}

	emails, total, err := db.ListEmails(context.Background(), database.EmailFilter{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(emails) != 1 {
		t.Fatalf("kept email missing: total=%d len=%d", total, len(emails))
	}
	if !emails[0].Kept {
		t.Fatal("email not marked kept")
	}

	if ok, err := db.DeleteEmail(context.Background(), id); err != nil || !ok {
		t.Fatalf("delete kept email: ok=%v err=%v", ok, err)
	}
}
