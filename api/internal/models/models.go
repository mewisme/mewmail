package models

import "time"

// Email is a stored inbound message.
type Email struct {
	ID          int64        `json:"id"`
	MessageID   string       `json:"message_id"`
	MailFrom    string       `json:"mail_from"`
	RcptTo      string       `json:"rcpt_to"`
	Subject     string       `json:"subject"`
	MailDate    string       `json:"mail_date"`
	TextBody    string       `json:"text_body,omitempty"`
	HTMLBody    string       `json:"html_body,omitempty"`
	HeadersJSON string       `json:"headers_json"`
	RawEmail    []byte       `json:"raw_email,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	OpenedAt    *time.Time   `json:"opened_at,omitempty"`
	Kept        bool         `json:"kept"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// Attachment holds metadata only; file bytes are not stored.
type Attachment struct {
	ID          int64  `json:"id"`
	EmailID     int64  `json:"email_id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

// EmailList is a paginated list response.
type EmailList struct {
	Emails []Email `json:"emails"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

// CredentialsFile is persisted at data/.credentials.
type CredentialsFile struct {
	APIKey      string    `json:"api_key"`      // external REST/UI
	InternalKey string    `json:"internal_key"` // Postfix ingest only
	CreatedAt   time.Time `json:"created_at"`
}
