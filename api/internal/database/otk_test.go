package database_test

import (
	"context"
	"testing"
	"time"

	"mewmail/api/internal/database"
	"mewmail/api/internal/models"
)

func TestConsumePreviewOTK_OneTime(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, otk, err := db.InsertEmail(context.Background(), &models.Email{
		MailFrom:    "a@x.com",
		RcptTo:      "b@x.com",
		Subject:     "test",
		HeadersJSON: "{}",
		RawEmail:    []byte("raw"),
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	ok, err := db.ConsumePreviewOTK(context.Background(), id, otk)
	if err != nil || !ok {
		t.Fatalf("first consume ok=%v err=%v", ok, err)
	}
	ok, err = db.ConsumePreviewOTK(context.Background(), id, otk)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("otk should be single-use")
	}
}

func TestKeepEmail_AlreadyKept(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, _, err := db.InsertEmail(context.Background(), &models.Email{
		MailFrom:    "a@x.com",
		RcptTo:      "b@x.com",
		Subject:     "test",
		HeadersJSON: "{}",
		RawEmail:    []byte("raw"),
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	status, ok, err := db.KeepEmail(context.Background(), id)
	if err != nil || !ok || status != "kept" {
		t.Fatalf("first keep status=%q ok=%v err=%v", status, ok, err)
	}
	status, ok, err = db.KeepEmail(context.Background(), id)
	if err != nil || !ok || status != "already_kept" {
		t.Fatalf("second keep status=%q ok=%v err=%v", status, ok, err)
	}
}

func TestValidPreviewOTK_DoesNotConsume(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, otk, err := db.InsertEmail(context.Background(), &models.Email{
		MailFrom:    "a@x.com",
		RcptTo:      "b@x.com",
		Subject:     "test",
		HeadersJSON: "{}",
		RawEmail:    []byte("raw"),
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	ok, err := db.ValidPreviewOTK(context.Background(), id, otk)
	if err != nil || !ok {
		t.Fatalf("valid otk ok=%v err=%v", ok, err)
	}
	ok, err = db.ConsumePreviewOTK(context.Background(), id, otk)
	if err != nil || !ok {
		t.Fatalf("preview should still consume otk ok=%v err=%v", ok, err)
	}
}

func TestTrackEmailOpen_FirstTimeOnly(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, _, err := db.InsertEmail(context.Background(), &models.Email{
		MailFrom:    "a@x.com",
		RcptTo:      "b@x.com",
		Subject:     "test",
		HeadersJSON: "{}",
		RawEmail:    []byte("raw"),
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	status, ok, err := db.TrackEmailOpen(context.Background(), id)
	if err != nil || !ok || status != "opened" {
		t.Fatalf("first open status=%q ok=%v err=%v", status, ok, err)
	}
	email, err := db.GetEmail(context.Background(), id)
	if err != nil || email.OpenedAt == nil {
		t.Fatalf("opened_at missing: err=%v", err)
	}

	status, ok, err = db.TrackEmailOpen(context.Background(), id)
	if err != nil || !ok || status != "already_opened" {
		t.Fatalf("second open status=%q ok=%v err=%v", status, ok, err)
	}
	first := email.OpenedAt
	email, err = db.GetEmail(context.Background(), id)
	if err != nil || email.OpenedAt == nil || !email.OpenedAt.Equal(*first) {
		t.Fatal("opened_at changed on second open")
	}
}
