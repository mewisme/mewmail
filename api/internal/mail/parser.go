package mail

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/mail"
	"strings"
	"time"

	gomail "github.com/emersion/go-message/mail"

	"mewmail/api/internal/models"
)

// ParseRaw parses RFC822 bytes into an Email model.
// When allowMultipart is false, messages with a multipart/* Content-Type are rejected.
func ParseRaw(raw []byte, rcptTo string, allowMultipart bool) (*models.Email, error) {
	mr, err := gomail.CreateReader(strings.NewReader(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("create reader: %w", err)
	}

	hdr := mr.Header
	if !allowMultipart {
		ct, _, _ := hdr.ContentType()
		if strings.HasPrefix(ct, "multipart/") {
			return nil, fmt.Errorf("multipart not allowed")
		}
	}
	headers := headerMap(hdr)

	msgID, _ := hdr.MessageID()
	fromList, _ := hdr.AddressList("From")
	toList, _ := hdr.AddressList("To")
	subject, _ := hdr.Subject()
	date, _ := hdr.Date()

	mailFrom := formatAddresses(fromList)
	mailTo := formatAddresses(toList)
	if mailTo == "" {
		mailTo = rcptTo
	}

	var textBody, htmlBody string
	var attachments []models.Attachment

	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("next part: %w", err)
		}

		ct := p.Header.Get("Content-Type")
		mediaType, params, _ := mime.ParseMediaType(ct)
		disp := p.Header.Get("Content-Disposition")
		dispType, dispParams, _ := mime.ParseMediaType(disp)

		body, err := io.ReadAll(p.Body)
		if err != nil {
			return nil, fmt.Errorf("read part: %w", err)
		}

		filename := params["name"]
		if filename == "" {
			filename = dispParams["filename"]
		}

		isAttachment := dispType == "attachment" || (filename != "" && !strings.HasPrefix(mediaType, "text/"))

		if isAttachment {
			attachments = append(attachments, models.Attachment{
				Filename:    filename,
				ContentType: mediaType,
				Size:        int64(len(body)),
			})
			continue
		}

		switch mediaType {
		case "text/plain":
			if textBody == "" {
				textBody = string(body)
			}
		case "text/html":
			if htmlBody == "" {
				htmlBody = string(body)
			}
		case "multipart/alternative", "multipart/mixed", "multipart/related":
			// nested handled by reader walking
		default:
			if filename != "" {
				attachments = append(attachments, models.Attachment{
					Filename:    filename,
					ContentType: mediaType,
					Size:        int64(len(body)),
				})
			}
		}
	}

	headersJSON, err := json.Marshal(headers)
	if err != nil {
		return nil, err
	}

	mailDateStr := ""
	if !date.IsZero() {
		mailDateStr = date.UTC().Format(time.RFC3339)
	}

	return &models.Email{
		MessageID:   msgID,
		MailFrom:    mailFrom,
		RcptTo:      rcptTo,
		Subject:     subject,
		MailDate:    mailDateStr,
		TextBody:    textBody,
		HTMLBody:    htmlBody,
		HeadersJSON: string(headersJSON),
		RawEmail:    raw,
		CreatedAt:   time.Now().UTC(),
		Attachments: attachments,
	}, nil
}

func headerMap(hdr gomail.Header) map[string][]string {
	return hdr.Map()
}

func formatAddresses(addrs []*mail.Address) string {
	if len(addrs) == 0 {
		return ""
	}
	parts := make([]string, len(addrs))
	for i, a := range addrs {
		if a.Name != "" {
			parts[i] = fmt.Sprintf("%s <%s>", a.Name, a.Address)
		} else {
			parts[i] = a.Address
		}
	}
	return strings.Join(parts, ", ")
}
