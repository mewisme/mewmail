package router

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"mewmail/api/internal/auth"
	"mewmail/api/internal/config"
	"mewmail/api/internal/database"
	"mewmail/api/internal/httputil"
	"mewmail/api/internal/mail"
	"mewmail/api/internal/middleware"
	"mewmail/api/internal/models"
	"mewmail/api/internal/webhook"
)

//go:embed openapi.yaml static/swagger/*
var openAPIFS embed.FS

var swaggerFS = openAPIFS

// Deps holds router dependencies.
type Deps struct {
	Config  *config.Config
	DB      *database.DB
	Log     *slog.Logger
	APIKey  string
	Webhook *webhook.Client
}

// New builds the HTTP router.
func New(d Deps) http.Handler {
	r := chi.NewRouter()

	timeout := time.Duration(d.Config.RequestTimeoutSec) * time.Second
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer(d.Log))
	r.Use(middleware.Timeout(timeout))
	r.Use(middleware.Logger(d.Log))
	r.Use(middleware.SecureHeaders)
	r.Use(middleware.BodyLimit(d.Config.MaxBodyBytes))
	r.Use(middleware.RateLimit(120, time.Minute))

	r.Get("/health", healthHandler)
	r.Get("/health/ready", readyHandler(d.DB))
	mountSwagger(r)

	ingest := mail.NewIngestHandler(d.DB, d.Log, d.Webhook)
	r.With(auth.InternalBearerAuth(d.APIKey, d.Log)).Post("/internal/ingest", ingest.ServeHTTP)

	h := &emailHandlers{
		db:             d.DB,
		log:            d.Log,
		apiKey:         d.APIKey,
		webhook:        d.Webhook,
		publicURL:      d.Config.PublicURL,
		requestTimeout: timeout,
	}
	r.Get("/emails/preview/{id}", h.preview)
	r.With(auth.BearerAuth(d.APIKey, d.Log)).Post("/emails/{id}/keep", h.keep)
	r.Get("/emails/{id}/keep", h.keep) // ponytail: GET for webhook one-click links only
	r.With(auth.BearerAuth(d.APIKey, d.Log)).Delete("/emails/{id}/keep", h.unkeep)
	r.With(auth.BearerAuth(d.APIKey, d.Log)).Post("/emails/{id}/preview-token", h.previewToken)
	r.With(auth.BearerAuth(d.APIKey, d.Log)).Get("/emails/{id}/raw", h.raw)

	r.Route("/emails", func(r chi.Router) {
		r.Use(auth.BearerAuth(d.APIKey, d.Log))
		r.Get("/", h.list)
		r.Get("/wait", h.wait)
		r.Get("/stats", h.stats)
		r.Get("/latest", h.latest)
		r.Head("/{id}", h.head)
		r.Get("/{id}", h.get)
		r.Delete("/", h.deleteMany)
		r.Delete("/{id}", h.deleteOne)
	})

	return r
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	httputil.WriteSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

func readyHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(r.Context()); err != nil {
			httputil.WriteError(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		httputil.WriteSuccess(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

type emailHandlers struct {
	db             *database.DB
	log            *slog.Logger
	apiKey         string
	webhook        *webhook.Client
	publicURL      string
	requestTimeout time.Duration
}

func (h *emailHandlers) list(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilter(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	emails, total, err := h.db.ListEmails(r.Context(), f)
	if err != nil {
		h.log.Error("list emails failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list emails")
		return
	}
	httputil.WriteSuccess(w, http.StatusOK, map[string]any{
		"emails": emails,
		"total":  total,
		"limit":  f.Limit,
		"offset": f.Offset,
	})
}

func (h *emailHandlers) latest(w http.ResponseWriter, r *http.Request) {
	limitParam := r.URL.Query().Get("limit")
	if limitParam == "" {
		emails, _, err := h.db.ListEmails(r.Context(), database.EmailFilter{Limit: 1})
		if err != nil {
			h.log.Error("get latest email failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "failed to get latest email")
			return
		}
		if len(emails) == 0 {
			httputil.WriteError(w, http.StatusNotFound, "no emails")
			return
		}
		httputil.WriteSuccess(w, http.StatusOK, emails[0])
		return
	}
	limit, err := strconv.Atoi(limitParam)
	if err != nil || limit < 1 {
		httputil.WriteError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	emails, total, err := h.db.ListEmails(r.Context(), database.EmailFilter{Limit: limit})
	if err != nil {
		h.log.Error("list latest emails failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list emails")
		return
	}
	httputil.WriteSuccess(w, http.StatusOK, map[string]any{
		"emails": emails,
		"total":  total,
		"limit":  limit,
	})
}

func (h *emailHandlers) wait(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilter(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	q := r.URL.Query()
	if v := q.Get("since_id"); v != "" {
		f.SinceID, err = strconv.ParseInt(v, 10, 64)
		if err != nil || f.SinceID < 0 {
			httputil.WriteError(w, http.StatusBadRequest, "invalid since_id")
			return
		}
	}
	timeout := 25 * time.Second
	if v := q.Get("timeout"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			httputil.WriteError(w, http.StatusBadRequest, "invalid timeout")
			return
		}
		timeout = time.Duration(n) * time.Second
	}
	maxWait := h.requestTimeout - 2*time.Second
	if maxWait < time.Second {
		maxWait = time.Second
	}
	if timeout > maxWait {
		timeout = maxWait
	}

	f.Limit = 1
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		emails, _, err := h.db.ListEmails(r.Context(), f)
		if err != nil {
			h.log.Error("wait for email failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "failed to wait for email")
			return
		}
		if len(emails) > 0 {
			httputil.WriteSuccess(w, http.StatusOK, emails[0])
			return
		}
		if time.Now().After(deadline) {
			httputil.WriteError(w, http.StatusRequestTimeout, "timeout waiting for email")
			return
		}
		select {
		case <-r.Context().Done():
			httputil.WriteError(w, http.StatusRequestTimeout, "timeout waiting for email")
			return
		case <-ticker.C:
		}
	}
}

func (h *emailHandlers) stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.db.EmailStats(r.Context())
	if err != nil {
		h.log.Error("email stats failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}
	httputil.WriteSuccess(w, http.StatusOK, stats)
}

func (h *emailHandlers) preview(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if !h.authorizeEmailLink(w, r, id, linkPreview) {
		return
	}
	if !h.trackFirstOpen(w, r, id, "preview") {
		return
	}

	email, err := h.db.GetEmail(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "email not found")
			return
		}
		h.log.Error("preview email failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get email")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data: https: http:; frame-src 'self'")
	_, _ = w.Write([]byte(renderEmailPreview(email)))
}

func (h *emailHandlers) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if trackOpenEnabled(r) {
		if !h.trackFirstOpen(w, r, id, "api") {
			return
		}
	}
	email, err := h.db.GetEmail(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "email not found")
			return
		}
		h.log.Error("get email failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get email")
		return
	}
	httputil.WriteSuccess(w, http.StatusOK, email)
}

func (h *emailHandlers) head(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ok, err := h.db.EmailExists(r.Context(), id)
	if err != nil {
		h.log.Error("head email failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to check email")
		return
	}
	if !ok {
		httputil.WriteError(w, http.StatusNotFound, "email not found")
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *emailHandlers) raw(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if trackOpenEnabled(r) {
		if !h.trackFirstOpen(w, r, id, "raw") {
			return
		}
	}
	email, err := h.db.GetEmail(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "email not found")
			return
		}
		h.log.Error("raw email failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get email")
		return
	}
	w.Header().Set("Content-Type", "message/rfc822")
	if email.MessageID != "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="email-%d.eml"`, id))
	}
	_, _ = w.Write(email.RawEmail)
}

func (h *emailHandlers) keep(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if r.Method == http.MethodGet && !h.authorizeEmailLink(w, r, id, linkKeep) {
		return
	}
	status, ok, err := h.db.KeepEmail(r.Context(), id)
	if err != nil {
		h.log.Error("keep email failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to keep email")
		return
	}
	if !ok {
		httputil.WriteError(w, http.StatusNotFound, "email not found")
		return
	}
	email, err := h.db.GetEmail(r.Context(), id)
	if err != nil {
		h.log.Error("get email after keep failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get email")
		return
	}
	if status == "kept" {
		h.log.Info("email kept", "id", id)
	}
	httputil.WriteSuccess(w, http.StatusOK, map[string]any{
		"status": status,
		"email":  email,
	})
}

func (h *emailHandlers) unkeep(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	status, ok, err := h.db.UnkeepEmail(r.Context(), id)
	if err != nil {
		h.log.Error("unkeep email failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to unkeep email")
		return
	}
	if !ok {
		httputil.WriteError(w, http.StatusNotFound, "email not found")
		return
	}
	email, err := h.db.GetEmail(r.Context(), id)
	if err != nil {
		h.log.Error("get email after unkeep failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get email")
		return
	}
	if status == "unkept" {
		h.log.Info("email unkept", "id", id)
	}
	httputil.WriteSuccess(w, http.StatusOK, map[string]any{
		"status": status,
		"email":  email,
	})
}

func (h *emailHandlers) previewToken(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	previewOTK, ok, err := h.db.RegeneratePreviewOTK(r.Context(), id)
	if err != nil {
		h.log.Error("regenerate preview token failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to regenerate preview token")
		return
	}
	if !ok {
		httputil.WriteError(w, http.StatusNotFound, "email not found")
		return
	}
	_, keepOTK, ok, err := h.db.GetLinkOTKs(r.Context(), id)
	if err != nil || !ok {
		h.log.Error("get link tokens failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get link tokens")
		return
	}
	data := map[string]any{"otk": previewOTK}
	if h.publicURL != "" {
		data["preview_url"] = fmt.Sprintf("%s/emails/preview/%d?otk=%s", h.publicURL, id, previewOTK)
		if keepOTK != "" {
			data["keep_url"] = fmt.Sprintf("%s/emails/%d/keep?otk=%s", h.publicURL, id, keepOTK)
		}
	}
	httputil.WriteSuccess(w, http.StatusOK, data)
}

func (h *emailHandlers) trackFirstOpen(w http.ResponseWriter, r *http.Request, id int64, via string) bool {
	status, ok, err := h.db.TrackEmailOpen(r.Context(), id)
	if err != nil {
		h.log.Error("track email open failed", "error", err, "id", id)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to track email open")
		return false
	}
	if !ok {
		httputil.WriteError(w, http.StatusNotFound, "email not found")
		return false
	}
	if status == "opened" {
		h.log.Info("email opened", "id", id, "via", via)
		if h.webhook != nil {
			email, err := h.db.GetEmail(r.Context(), id)
			if err == nil && email.OpenedAt != nil {
				h.webhook.EmailOpened(email.ID, email.MailFrom, email.RcptTo, email.Subject, email.MessageID, via, *email.OpenedAt)
			}
		}
	}
	return true
}

func (h *emailHandlers) authorizeEmailLink(w http.ResponseWriter, r *http.Request, id int64, purpose linkPurpose) bool {
	q := r.URL.Query()
	switch {
	case auth.ValidAPIKey(q.Get("apikey"), h.apiKey):
		return true
	case q.Get("otk") != "":
		var ok bool
		var err error
		switch purpose {
		case linkPreview:
			ok, err = h.db.ConsumePreviewOTK(r.Context(), id, q.Get("otk"))
		case linkKeep:
			ok, err = h.db.ValidKeepOTK(r.Context(), id, q.Get("otk"))
		default:
			httputil.WriteError(w, http.StatusInternalServerError, "failed to authorize")
			return false
		}
		if err != nil {
			h.log.Error("email link auth failed", "error", err, "purpose", purpose)
			httputil.WriteError(w, http.StatusInternalServerError, "failed to authorize")
			return false
		}
		if !ok {
			h.log.Warn("email link auth failure", "path", r.URL.Path, "remote", r.RemoteAddr)
			httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return false
		}
		return true
	default:
		h.log.Warn("email link auth failure", "path", r.URL.Path, "remote", r.RemoteAddr)
		httputil.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
}

type linkPurpose string

const (
	linkPreview linkPurpose = "preview"
	linkKeep    linkPurpose = "keep"
)

func (h *emailHandlers) deleteOne(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ok, err := h.db.DeleteEmail(r.Context(), id)
	if err != nil {
		h.log.Error("delete email failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to delete email")
		return
	}
	if !ok {
		httputil.WriteError(w, http.StatusNotFound, "email not found")
		return
	}
	h.log.Info("email deleted", "id", id)
	httputil.WriteSuccess(w, http.StatusOK, map[string]any{"deleted": 1})
}

func (h *emailHandlers) deleteMany(w http.ResponseWriter, r *http.Request) {
	f, err := parseFilter(r)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	n, err := h.db.DeleteEmails(r.Context(), f)
	if err != nil {
		h.log.Error("delete emails failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to delete emails")
		return
	}
	h.log.Info("emails deleted", "count", n)
	httputil.WriteSuccess(w, http.StatusOK, map[string]any{"deleted": n})
}

func parseFilter(r *http.Request) (database.EmailFilter, error) {
	q := r.URL.Query()
	f := database.EmailFilter{
		From:      q.Get("from"),
		To:        q.Get("to"),
		Subject:   q.Get("subject"),
		MessageID: q.Get("message_id"),
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return f, errInvalid("limit")
		}
		f.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return f, errInvalid("offset")
		}
		f.Offset = n
	}
	if v := q.Get("after"); v != "" {
		t, err := parseTime(v)
		if err != nil {
			return f, errInvalid("after")
		}
		f.After = &t
	}
	if v := q.Get("before"); v != "" {
		t, err := parseTime(v)
		if err != nil {
			return f, errInvalid("before")
		}
		f.Before = &t
	}
	if v := q.Get("kept"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return f, errInvalid("kept")
		}
		f.Kept = &b
	}
	if v := q.Get("opened"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return f, errInvalid("opened")
		}
		f.Opened = &b
	}
	return f, nil
}

func trackOpenEnabled(r *http.Request) bool {
	return r.URL.Query().Get("track_open") != "false"
}

func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", s)
}

func errInvalid(field string) error {
	return &filterError{field: field}
}

type filterError struct{ field string }

func (e *filterError) Error() string {
	return "invalid " + e.field
}

func renderEmailPreview(e *models.Email) string {
	var b strings.Builder
	title := e.Subject
	if title == "" {
		title = fmt.Sprintf("Email #%d", e.ID)
	}
	fmt.Fprintf(&b, `<!DOCTYPE html>
<html lang="en"><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>
body{font-family:system-ui,sans-serif;margin:0 auto;padding:1rem 1.5rem;max-width:960px;color:#111}
.meta{border-bottom:1px solid #ddd;padding-bottom:1rem;margin-bottom:1rem}
.meta dt{font-weight:600;color:#555;margin-top:.5rem}
.meta dd{margin:0}
iframe{width:100%%;min-height:480px;border:1px solid #ddd;border-radius:4px}
pre{white-space:pre-wrap;word-break:break-word;background:#f6f6f6;padding:1rem;border-radius:4px}
.attachments{margin-top:1rem;font-size:.9rem;color:#555}
.empty{color:#888;font-style:italic}
</style>
</head><body>
<h1>%s</h1>
<dl class="meta">
<dt>From</dt><dd>%s</dd>
<dt>To</dt><dd>%s</dd>
<dt>Date</dt><dd>%s</dd>
</dl>
`, html.EscapeString(title), html.EscapeString(title),
		html.EscapeString(e.MailFrom), html.EscapeString(e.RcptTo), html.EscapeString(e.MailDate))

	switch {
	case e.HTMLBody != "":
		fmt.Fprintf(&b, `<iframe sandbox="" srcdoc="%s"></iframe>`, html.EscapeString(e.HTMLBody))
	case e.TextBody != "":
		fmt.Fprintf(&b, `<pre>%s</pre>`, html.EscapeString(e.TextBody))
	default:
		b.WriteString(`<p class="empty">(no body)</p>`)
	}

	if len(e.Attachments) > 0 {
		b.WriteString(`<div class="attachments"><strong>Attachments:</strong><ul>`)
		for _, a := range e.Attachments {
			fmt.Fprintf(&b, `<li>%s (%s, %d bytes)</li>`,
				html.EscapeString(a.Filename), html.EscapeString(a.ContentType), a.Size)
		}
		b.WriteString(`</ul></div>`)
	}

	b.WriteString(`</body></html>`)
	return b.String()
}
