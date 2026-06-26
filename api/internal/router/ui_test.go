package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mewmail/api/internal/models"
)

func TestUIIndexHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	uiIndexHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "MewMail") || !strings.Contains(body, "/ui/static/app.js") {
		t.Fatalf("unexpected body: %s", body[:min(200, len(body))])
	}
	if !strings.Contains(body, "/ui/static/ui.css") {
		t.Fatal("missing ui.css link")
	}
	if !strings.Contains(body, "cdn.jsdelivr.net/npm/bootstrap@5.3.3") {
		t.Fatal("missing Bootstrap CDN")
	}
	if !strings.Contains(body, "bootstrap.bundle.min.js") {
		t.Fatal("missing Bootstrap JS CDN")
	}
	if !strings.Contains(body, "dompurify") {
		t.Fatal("missing DOMPurify CDN")
	}
	if !strings.Contains(rr.Header().Get("Content-Security-Policy"), "script-src 'self' https:") {
		t.Fatalf("missing CDN script CSP, got %q", rr.Header().Get("Content-Security-Policy"))
	}
}

func TestServePreviewHTML(t *testing.T) {
	email := &models.Email{
		ID:       3,
		Subject:  "Hello",
		MailFrom: "a@x.com",
		RcptTo:   "b@x.com",
		MailDate: time.Now().Format(time.RFC3339),
		TextBody: "plain text",
	}
	rr := httptest.NewRecorder()
	servePreviewHTML(rr, email)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{`"subject":"Hello"`, "plain text", "/ui/static/preview.js", "email-data"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q", want)
		}
	}
	start := strings.Index(body, `<script id="email-data"`)
	if start < 0 {
		t.Fatal("email-data script missing")
	}
	end := strings.Index(body[start:], "</script>")
	if end < 0 {
		t.Fatal("email-data script not closed")
	}
	jsonPart := body[start : start+end]
	if strings.Contains(jsonPart, "</script>") {
		t.Fatal("unescaped </script> inside email-data script")
	}
}
