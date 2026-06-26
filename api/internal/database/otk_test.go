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

	id, previewOTK, _, err := db.InsertEmail(context.Background(), &models.Email{
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

	ok, err := db.ConsumePreviewOTK(context.Background(), id, previewOTK)
	if err != nil || !ok {
		t.Fatalf("first consume ok=%v err=%v", ok, err)
	}
	ok, err = db.ConsumePreviewOTK(context.Background(), id, previewOTK)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("otk should be single-use")
	}
}

func TestConsumePreviewOTK_KeepsKeepOTK(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, previewOTK, keepOTK, err := db.InsertEmail(context.Background(), &models.Email{
		MailFrom: "a@x.com", RcptTo: "b@x.com", Subject: "test",
		HeadersJSON: "{}", RawEmail: []byte("raw"), CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	ok, err := db.ConsumePreviewOTK(context.Background(), id, previewOTK)
	if err != nil || !ok {
		t.Fatalf("consume preview ok=%v err=%v", ok, err)
	}
	ok, err = db.ValidPreviewOTK(context.Background(), id, previewOTK)
	if err != nil || ok {
		t.Fatalf("preview otk should be gone ok=%v err=%v", ok, err)
	}
	ok, err = db.ValidKeepOTK(context.Background(), id, keepOTK)
	if err != nil || !ok {
		t.Fatalf("keep otk should still work ok=%v err=%v", ok, err)
	}
}

func TestKeepEmail_AlreadyKept(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, _, _, err := db.InsertEmail(context.Background(), &models.Email{
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

	id, previewOTK, _, err := db.InsertEmail(context.Background(), &models.Email{
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

	ok, err := db.ValidPreviewOTK(context.Background(), id, previewOTK)
	if err != nil || !ok {
		t.Fatalf("valid otk ok=%v err=%v", ok, err)
	}
	ok, err = db.ConsumePreviewOTK(context.Background(), id, previewOTK)
	if err != nil || !ok {
		t.Fatalf("preview should still consume otk ok=%v err=%v", ok, err)
	}
}

func TestListEmails_KeptFilter(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	id1, _, _, err := db.InsertEmail(ctx, &models.Email{
		MailFrom: "a@x.com", RcptTo: "b@x.com", Subject: "one",
		HeadersJSON: "{}", RawEmail: []byte("raw"), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	id2, _, _, err := db.InsertEmail(ctx, &models.Email{
		MailFrom: "a@x.com", RcptTo: "b@x.com", Subject: "two",
		HeadersJSON: "{}", RawEmail: []byte("raw"), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := db.KeepEmail(ctx, id1); err != nil || !ok {
		t.Fatalf("keep id1 ok=%v err=%v", ok, err)
	}

	keptTrue := true
	keptFalse := false
	emails, total, err := db.ListEmails(ctx, database.EmailFilter{Kept: &keptTrue, Limit: 50})
	if err != nil || total != 1 || len(emails) != 1 || emails[0].ID != id1 || !emails[0].Kept {
		t.Fatalf("kept=true: total=%d len=%d id=%d kept=%v err=%v", total, len(emails), emails[0].ID, emails[0].Kept, err)
	}
	emails, total, err = db.ListEmails(ctx, database.EmailFilter{Kept: &keptFalse, Limit: 50})
	if err != nil || total != 1 || len(emails) != 1 || emails[0].ID != id2 || emails[0].Kept {
		t.Fatalf("kept=false: total=%d len=%d id=%d kept=%v err=%v", total, len(emails), emails[0].ID, emails[0].Kept, err)
	}
	emails, total, err = db.ListEmails(ctx, database.EmailFilter{Limit: 50})
	if err != nil || total != 2 || len(emails) != 2 {
		t.Fatalf("no filter: total=%d len=%d err=%v", total, len(emails), err)
	}
}

func TestUnkeepEmail(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	id, _, _, err := db.InsertEmail(ctx, &models.Email{
		MailFrom: "a@x.com", RcptTo: "b@x.com", Subject: "test",
		HeadersJSON: "{}", RawEmail: []byte("raw"), CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := db.KeepEmail(ctx, id); err != nil || !ok {
		t.Fatalf("keep ok=%v err=%v", ok, err)
	}
	status, ok, err := db.UnkeepEmail(ctx, id)
	if err != nil || !ok || status != "unkept" {
		t.Fatalf("first unkeep status=%q ok=%v err=%v", status, ok, err)
	}
	status, ok, err = db.UnkeepEmail(ctx, id)
	if err != nil || !ok || status != "already_unkept" {
		t.Fatalf("second unkeep status=%q ok=%v err=%v", status, ok, err)
	}
}

func TestListEmails_MessageIDAndOpenedFilters(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	id1, _, _, err := db.InsertEmail(ctx, &models.Email{
		MessageID: "<one@x.com>", MailFrom: "a@x.com", RcptTo: "b@x.com", Subject: "one",
		HeadersJSON: "{}", RawEmail: []byte("raw"), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = db.InsertEmail(ctx, &models.Email{
		MessageID: "<two@x.com>", MailFrom: "a@x.com", RcptTo: "b@x.com", Subject: "two",
		HeadersJSON: "{}", RawEmail: []byte("raw"), CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := db.TrackEmailOpen(ctx, id1); err != nil || !ok {
		t.Fatalf("open id1 ok=%v err=%v", ok, err)
	}

	emails, total, err := db.ListEmails(ctx, database.EmailFilter{MessageID: "<one@x.com>", Limit: 50})
	if err != nil || total != 1 || len(emails) != 1 || emails[0].ID != id1 {
		t.Fatalf("message_id filter: total=%d len=%d err=%v", total, len(emails), err)
	}

	openedTrue := true
	openedFalse := false
	emails, total, err = db.ListEmails(ctx, database.EmailFilter{Opened: &openedTrue, Limit: 50})
	if err != nil || total != 1 || len(emails) != 1 || emails[0].ID != id1 {
		t.Fatalf("opened=true: total=%d len=%d err=%v", total, len(emails), err)
	}
	emails, total, err = db.ListEmails(ctx, database.EmailFilter{Opened: &openedFalse, Limit: 50})
	if err != nil || total != 1 || len(emails) != 1 {
		t.Fatalf("opened=false: total=%d len=%d err=%v", total, len(emails), err)
	}
}

func TestEmailStatsAndRegenerateOTK(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	id, otk1, _, err := db.InsertEmail(ctx, &models.Email{
		MailFrom: "a@x.com", RcptTo: "b@x.com", Subject: "test",
		HeadersJSON: "{}", RawEmail: []byte("raw"), CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	stats, err := db.EmailStats(ctx)
	if err != nil || stats.Total != 1 || stats.Unopened != 1 {
		t.Fatalf("stats total=%d unopened=%d err=%v", stats.Total, stats.Unopened, err)
	}

	otk2, ok, err := db.RegeneratePreviewOTK(ctx, id)
	if err != nil || !ok || otk2 == "" || otk2 == otk1 {
		t.Fatalf("regen otk=%q ok=%v err=%v", otk2, ok, err)
	}
	ok, err = db.ValidPreviewOTK(ctx, id, otk1)
	if err != nil || ok {
		t.Fatalf("old otk should be invalid ok=%v err=%v", ok, err)
	}
	ok, err = db.ValidPreviewOTK(ctx, id, otk2)
	if err != nil || !ok {
		t.Fatalf("new otk ok=%v err=%v", ok, err)
	}
}

func TestTrackEmailOpen_FirstTimeOnly(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, _, _, err := db.InsertEmail(context.Background(), &models.Email{
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
