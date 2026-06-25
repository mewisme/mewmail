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
	avatarURL      = "https://github.com/mewisme.png"
	requestTimeout = 10 * time.Second
)

// Client posts event notifications to WEBHOOK_URL.
type Client struct {
	url       string
	publicURL string
	http      *http.Client
	log       *slog.Logger
	discord   bool
}

// New returns a webhook client. Empty url disables notifications.
func New(url, publicURL string, log *slog.Logger) *Client {
	return &Client{
		url:       strings.TrimSpace(url),
		publicURL: strings.TrimRight(strings.TrimSpace(publicURL), "/"),
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
func (c *Client) EmailReceived(id int64, from, to, subject, messageID, previewOTK string) {
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
	if previewURL := c.previewURL(id, previewOTK); previewURL != "" {
		data["preview_url"] = previewURL
	}
	if keepURL := c.keepURL(id, previewOTK); keepURL != "" {
		data["keep_url"] = keepURL
	}
	if c.discord {
		go c.postDiscord(c.emailReceivedEmbed(id, from, to, subject, messageID, previewOTK, now))
		return
	}
	go c.postGeneric("email.received", now, data)
}

// EmailOpened notifies that an email was opened for the first time.
func (c *Client) EmailOpened(id int64, from, to, subject, messageID, via string, openedAt time.Time) {
	if !c.Enabled() {
		return
	}
	at := openedAt.UTC()
	data := map[string]any{
		"id":         id,
		"from":       from,
		"to":         to,
		"subject":    subject,
		"message_id": messageID,
		"opened_at":  at.Format(time.RFC3339),
		"via":        via,
	}
	if c.discord {
		go c.postDiscord(c.emailOpenedEmbed(id, from, to, subject, via, at))
		return
	}
	go c.postGeneric("email.opened", at, data)
}

func (c *Client) previewURL(id int64, otk string) string {
	if c.publicURL == "" || otk == "" {
		return ""
	}
	return fmt.Sprintf("%s/emails/preview/%d?otk=%s", c.publicURL, id, otk)
}

func (c *Client) keepURL(id int64, otk string) string {
	if c.publicURL == "" || otk == "" {
		return ""
	}
	return fmt.Sprintf("%s/emails/%d/keep?otk=%s", c.publicURL, id, otk)
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
	if c.discord {
		go c.postDiscord(c.emailsCleanedEmbed(count, cutoff, retentionHours, now))
		return
	}
	go c.postGeneric("email.cleaned", now, data)
}

func (c *Client) emailReceivedEmbed(id int64, from, to, subject, messageID, previewOTK string, at time.Time) discordEmbed {
	fields := []discordField{
		{Name: "ID", Value: fmt.Sprintf("`#%d`", id), Inline: true},
	}
	if messageID != "" {
		fields = append(fields, discordField{
			Name:   "Message-ID",
			Value:  codeQuote(truncate(messageID, 200)),
			Inline: false,
		})
	}
	if actions := c.actionLinks(id, previewOTK); actions != "" {
		fields = append(fields, discordField{Name: "Actions", Value: actions, Inline: false})
	}
	return discordEmbed{
		Title: "New email",
		Description: embedBody(subject,
			fmt.Sprintf("**%s** → **%s**", from, to),
		),
		Color:       0x57F287,
		Timestamp:   at.Format(time.RFC3339),
		Footer:      newDiscordFooter("New email"),
		Fields:      fields,
	}
}

func (c *Client) emailOpenedEmbed(id int64, from, to, subject, via string, at time.Time) discordEmbed {
	return discordEmbed{
		Title: "Email opened",
		Description: embedBody(subject,
			fmt.Sprintf("First opened via **%s**", formatVia(via)),
		),
		Color:     0x5865F2,
		Timestamp: at.Format(time.RFC3339),
		Footer:    newDiscordFooter("Email opened"),
		Fields: []discordField{
			{Name: "From", Value: from, Inline: true},
			{Name: "To", Value: to, Inline: true},
			{Name: "ID", Value: fmt.Sprintf("`#%d`", id), Inline: true},
			{Name: "Opened", Value: formatTime(at), Inline: false},
		},
	}
}

func (c *Client) emailsCleanedEmbed(count int64, cutoff time.Time, retentionHours int, at time.Time) discordEmbed {
	return discordEmbed{
		Title:       "Retention cleanup",
		Description: fmt.Sprintf("**%d** email(s) removed", count),
		Color:       0xFEE75C,
		Timestamp:   at.Format(time.RFC3339),
		Footer:      newDiscordFooter("Cleanup"),
		Fields: []discordField{
			{Name: "Older than", Value: formatTime(cutoff), Inline: true},
			{Name: "Retention", Value: fmt.Sprintf("%d hours", retentionHours), Inline: true},
		},
	}
}

func (c *Client) actionLinks(id int64, otk string) string {
	var links []string
	if u := c.previewURL(id, otk); u != "" {
		links = append(links, discordLink("Open", u))
	}
	if u := c.keepURL(id, otk); u != "" {
		links = append(links, discordLink("Keep", u))
	}
	return strings.Join(links, " · ")
}

func (c *Client) postDiscord(embed discordEmbed) {
	body, err := json.Marshal(discordPayload{
		Username:  username,
		AvatarURL: avatarURL,
		Embeds:    []discordEmbed{embed},
	})
	if err != nil {
		c.log.Error("webhook marshal failed", "error", err)
		return
	}
	c.post(body)
}

func (c *Client) postGeneric(event string, at time.Time, data map[string]any) {
	body, err := json.Marshal(map[string]any{
		"event":     event,
		"timestamp": at.Format(time.RFC3339),
		"data":      data,
	})
	if err != nil {
		c.log.Error("webhook marshal failed", "event", event, "error", err)
		return
	}
	c.post(body)
}

func (c *Client) post(body []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		c.log.Error("webhook request failed", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		c.log.Error("webhook post failed", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.log.Error("webhook bad status", "status", resp.StatusCode)
	}
}

type discordPayload struct {
	Username  string         `json:"username"`
	AvatarURL string         `json:"avatar_url,omitempty"`
	Embeds    []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	Color       int            `json:"color,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
	Fields      []discordField `json:"fields,omitempty"`
	Footer      *discordFooter `json:"footer,omitempty"`
}

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type discordFooter struct {
	Text string `json:"text"`
}

func newDiscordFooter(event string) *discordFooter {
	return &discordFooter{Text: username + " · " + event}
}

func discordLink(label, url string) string {
	return fmt.Sprintf("[%s](%s)", label, url)
}

func displaySubject(subject string) string {
	if s := strings.TrimSpace(subject); s != "" {
		return truncate(s, 256)
	}
	return "(no subject)"
}

func embedBody(subject string, lines ...string) string {
	parts := append([]string{"**" + displaySubject(subject) + "**"}, lines...)
	return strings.Join(parts, "\n\n")
}

func formatVia(via string) string {
	switch via {
	case "preview":
		return "Preview link"
	case "api":
		return "API"
	default:
		return via
	}
}

func formatTime(t time.Time) string {
	return t.UTC().Format("2 Jan 2006, 15:04 UTC")
}

func codeQuote(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "'") + "`"
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
