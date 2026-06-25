package router

import (
	"database/sql"
	"embed"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"mewmail/api/internal/auth"
	"mewmail/api/internal/config"
	"mewmail/api/internal/database"
	"mewmail/api/internal/httputil"
	"mewmail/api/internal/mail"
	"mewmail/api/internal/middleware"
	"mewmail/api/internal/webhook"
)

//go:embed openapi.yaml
var openAPIFS embed.FS

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
	r.Get("/swagger", swaggerUIHandler)
	r.Get("/swagger/", swaggerUIHandler)
	r.Get("/swagger/init.js", swaggerInitHandler)
	r.Get("/swagger/openapi.yaml", openAPIHandler)

	ingest := mail.NewIngestHandler(d.DB, d.Log, d.Webhook)
	r.With(auth.InternalBearerAuth(d.APIKey, d.Log)).Post("/internal/ingest", ingest.ServeHTTP)

	r.Route("/emails", func(r chi.Router) {
		r.Use(auth.BearerAuth(d.APIKey, d.Log))
		h := &emailHandlers{db: d.DB, log: d.Log}
		r.Get("/", h.list)
		r.Get("/latest", h.latest)
		r.Get("/{id}", h.get)
		r.Delete("/", h.deleteMany)
		r.Delete("/{id}", h.deleteOne)
	})

	return r
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	httputil.WriteSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
}

func swaggerUIHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self' https://cdn.jsdelivr.net https://static.cloudflareinsights.com; style-src 'self' https://cdn.jsdelivr.net 'unsafe-inline'; img-src 'self' data: https://cdn.jsdelivr.net; font-src 'self' https://cdn.jsdelivr.net; connect-src 'self' https://cdn.jsdelivr.net")
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang="en"><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>MewMailAPI</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.11.0/swagger-ui.css">
</head><body>
<div id="swagger-ui"></div>
<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.11.0/swagger-ui-bundle.js" crossorigin></script>
<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.11.0/swagger-ui-standalone-preset.js" crossorigin></script>
<script src="/swagger/init.js"></script>
</body></html>`))
}

func swaggerInitHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write([]byte(`window.onload = function() {
  if (!window.SwaggerUIBundle) {
    document.body.innerHTML = "SwaggerUIBundle not loaded";
    return;
  }
  SwaggerUIBundle({
    url: window.location.origin + "/swagger/openapi.yaml",
    dom_id: "#swagger-ui",
    deepLinking: true,
    presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
    layout: "StandaloneLayout"
  });
};`))
}

func openAPIHandler(w http.ResponseWriter, _ *http.Request) {
	data, err := fs.ReadFile(openAPIFS, "openapi.yaml")
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "spec not found")
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(data)
}

type emailHandlers struct {
	db  *database.DB
	log *slog.Logger
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

func (h *emailHandlers) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid id")
		return
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
		From:    q.Get("from"),
		To:      q.Get("to"),
		Subject: q.Get("subject"),
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
	return f, nil
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
