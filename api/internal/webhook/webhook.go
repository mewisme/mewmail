package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	username       = "MewMail"
	requestTimeout = 10 * time.Second
)

// Client posts event notifications to WEBHOOK_URL.
type Client struct {
	url     string
	http    *http.Client
	log     *slog.Logger
	discord bool
}

// New returns a webhook client. Empty url disables notifications.
func New(url string, log *slog.Logger) *Client {
	return &Client{
		url: strings.TrimSpace(url),
		http: &http.Client{
			Timeout: requestTimeout,
		},
		log:     log,
		discord: strings.Contains(url, "/api/webhooks/"),
	}
}

// Enabled reports whether webhooks are configured.
func (c *Client) Enabled() bool {
	return c != nil && c.url != ""
}

// EmailReceived notifies that a new email was stored.
func (c *Client) EmailReceived(id int64, from, to, subject, messageID string) {
	if !c.Enabled() {
		return
	}
	now := time.Now().UTC()
	data := map[string]any{
		"id":         id,
		"from":       from,
		"to":         to,
		"subject":    subject,
		"message_id": messageID,
	}
	go c.send("email.received", "Email received", 0x57F287, now, []field{
		{"ID", fmt.Sprintf("%d", id), true},
		{"From", from, true},
		{"To", to, true},
		{"Subject", truncate(subject, 256), false},
		{"Message-ID", truncate(messageID, 256), false},
	}, data)
}

// EmailsCleaned notifies that expired emails were deleted.
func (c *Client) EmailsCleaned(count int64, cutoff time.Time, retentionHours int) {
	if !c.Enabled() || count == 0 {
		return
	}
	now := time.Now().UTC()
	data := map[string]any{
		"count":           count,
		"cutoff":          cutoff.Format(time.RFC3339),
		"retention_hours": retentionHours,
	}
	go c.send("email.cleaned", "Emails cleaned", 0xFEE75C, now, []field{
		{"Deleted", fmt.Sprintf("%d", count), true},
		{"Older than", cutoff.Format(time.RFC3339), true},
		{"Retention", fmt.Sprintf("%d hours", retentionHours), true},
	}, data)
}

type field struct {
	name   string
	value  string
	inline bool
}

func (c *Client) send(event, title string, color int, at time.Time, fields []field, data map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	var body []byte
	var err error
	if c.discord {
		body, err = json.Marshal(buildDiscordPayload(title, color, at, fields))
	} else {
		body, err = json.Marshal(map[string]any{
			"event":     event,
			"timestamp": at.Format(time.RFC3339),
			"data":      data,
		})
	}
	if err != nil {
		c.log.Error("webhook marshal failed", "event", event, "error", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		c.log.Error("webhook request failed", "event", event, "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		c.log.Error("webhook post failed", "event", event, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.log.Error("webhook bad status", "event", event, "status", resp.StatusCode)
	}
}

type discordPayload struct {
	Username string         `json:"username"`
	Embeds   []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title     string         `json:"title"`
	Color     int            `json:"color"`
	Timestamp string         `json:"timestamp"`
	Fields    []discordField `json:"fields"`
}

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

func buildDiscordPayload(title string, color int, at time.Time, fields []field) discordPayload {
	embedFields := make([]discordField, len(fields))
	for i, f := range fields {
		embedFields[i] = discordField{
			Name:   f.name,
			Value:  truncate(f.value, 1024),
			Inline: f.inline,
		}
	}
	return discordPayload{
		Username: username,
		Embeds: []discordEmbed{{
			Title:     title,
			Color:     color,
			Timestamp: at.Format(time.RFC3339),
			Fields:    embedFields,
		}},
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
