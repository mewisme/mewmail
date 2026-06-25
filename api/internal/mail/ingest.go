package mail

import (
	"io"
	"log/slog"
	"net/http"

	"mewmail/api/internal/database"
	"mewmail/api/internal/httputil"
)

// IngestHandler handles POST /internal/ingest from Postfix.
type IngestHandler struct {
	DB             *database.DB
	Log            *slog.Logger
	AllowMultipart bool
}

// ServeHTTP ingests a raw RFC822 message.
func (h *IngestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rcptTo := r.Header.Get("X-Envelope-To")
	if rcptTo == "" {
		httputil.WriteError(w, http.StatusBadRequest, "missing X-Envelope-To header")
		return
	}

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	if len(raw) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "empty body")
		return
	}

	email, err := ParseRaw(raw, rcptTo, h.AllowMultipart)
	if err != nil {
		h.Log.Error("parse email failed", "error", err, "request_id", r.Header.Get("X-Request-Id"))
		httputil.WriteError(w, http.StatusBadRequest, "failed to parse email")
		return
	}

	id, err := h.DB.InsertEmail(r.Context(), email)
	if err != nil {
		h.Log.Error("insert email failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to store email")
		return
	}

	h.Log.Info("email received",
		"id", id,
		"from", email.MailFrom,
		"to", email.RcptTo,
		"subject", email.Subject,
		"message_id", email.MessageID,
	)

	httputil.WriteSuccess(w, http.StatusCreated, map[string]any{"id": id})
}

// NewIngestHandler returns the ingest HTTP handler.
func NewIngestHandler(db *database.DB, log *slog.Logger, allowMultipart bool) http.Handler {
	return &IngestHandler{DB: db, Log: log, AllowMultipart: allowMultipart}
}
