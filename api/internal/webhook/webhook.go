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
func (c *Client) EmailReceived(id int64, from, to, subject, messageID, previewOTK, keepOTK string) {
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
	if keepURL := c.keepURL(id, keepOTK); keepURL != "" {
		data["keep_url"] = keepURL
	}
	if c.discord {
		go c.postDiscord(c.emailReceivedEmbed(id, from, to, subject, messageID, previewOTK, keepOTK, now))
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
	return fmt.Sprintf("%s/preview/%d?otk=%s", c.publicURL, id, otk)
}

func (c *Client) keepURL(id int64, otk string) string {
	if c.publicURL == "" || otk == "" {
		return ""
	}
	return fmt.Sprintf("%s/api/emails/%d/keep?otk=%s", c.publicURL, id, otk)
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

func (c *Client) emailReceivedEmbed(id int64, from, to, subject, messageID, previewOTK, keepOTK string, at time.Time) discordEmbed {
	unixTime := at.Unix()
	
	fields := []discordField{
		{Name: "Sender", Value: fmt.Sprintf("`%s`", from), Inline: true},
		{Name: "Recipient", Value: fmt.Sprintf("`%s`", to), Inline: true},
		{Name: "Received At", Value: fmt.Sprintf("<t:%d:F> (<t:%d:R>)", unixTime, unixTime), Inline: false},
	}

	if messageID != "" {
		fields = append(fields, discordField{
			Name:   "Message ID",
			Value:  codeQuote(truncate(messageID, 200)),
			Inline: false,
		})
	}

	if actions := c.actionLinks(id, previewOTK, keepOTK); actions != "" {
		fields = append(fields, discordField{
			Name:   "Actions",
			Value:  "» " + actions,
			Inline: false,
		})
	}

	return discordEmbed{
		Title:       fmt.Sprintf("Email Inbound | #%d", id),
		Description: fmt.Sprintf("> %s", displaySubject(subject)),
		Color:       0x57F287, // Green (Xanh lá cây thành công)
		Timestamp:   at.Format(time.RFC3339),
		Fields:      fields,
		Footer:      newDiscordFooter("Inbound Notification"),
	}
}

func (c *Client) emailOpenedEmbed(id int64, from, to, subject, via string, at time.Time) discordEmbed {
	unixTime := at.Unix()

	return discordEmbed{
		Title:       fmt.Sprintf("Email Opened | #%d", id),
		Description: fmt.Sprintf("> %s\n\nOpened via: **%s**", displaySubject(subject), formatVia(via)),
		Color:       0x5865F2, // Blurple (Xanh dương Discord)
		Timestamp:   at.Format(time.RFC3339),
		Fields: []discordField{
			{Name: "Sender", Value: fmt.Sprintf("`%s`", from), Inline: true},
			{Name: "Recipient", Value: fmt.Sprintf("`%s`", to), Inline: true},
			{Name: "Opened At", Value: fmt.Sprintf("<t:%d:F> (<t:%d:R>)", unixTime, unixTime), Inline: false},
		},
		Footer: newDiscordFooter("Telemetry Event"),
	}
}

func (c *Client) emailsCleanedEmbed(count int64, cutoff time.Time, retentionHours int, at time.Time) discordEmbed {
	unixCutoff := cutoff.Unix()

	return discordEmbed{
		Title:       "Data Retention Cleanup",
		Description: fmt.Sprintf("> Automated storage optimization successfully completed.\n\n**Status:** Completed\n**Purged:** %d email(s)", count),
		Color:       0xFEE75C, // Yellow (Vàng cảnh báo hệ thống)
		Timestamp:   at.Format(time.RFC3339),
		Fields: []discordField{
			{Name: "Retention Policy", Value: fmt.Sprintf("%d hours", retentionHours), Inline: true},
			{Name: "Cutoff Threshold", Value: fmt.Sprintf("<t:%d:F>", unixCutoff), Inline: true},
		},
		Footer:      newDiscordFooter("System Maintenance"),
	}
}

func (c *Client) actionLinks(id int64, previewOTK, keepOTK string) string {
	var links []string
	if u := c.previewURL(id, previewOTK); u != "" {
		links = append(links, discordLink("Open", u))
	}
	if u := c.keepURL(id, keepOTK); u != "" {
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