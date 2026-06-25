package mail_test

import (
	"strings"
	"testing"

	"mewmail/api/internal/mail"
)

const plainEmail = "From: sender@example.com\r\n" +
	"To: recv@example.com\r\n" +
	"Subject: Hello World\r\n" +
	"Message-ID: <test123@example.com>\r\n" +
	"Date: Mon, 02 Jan 2006 15:04:05 +0000\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"Plain body text.\r\n"

const multipartEmail = "From: multi@example.com\r\n" +
	"To: recv@example.com\r\n" +
	"Subject: Multipart =?UTF-8?B?4piF?= test\r\n" +
	"Message-ID: <multi@example.com>\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=bound\r\n" +
	"\r\n" +
	"--bound\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"Text part\r\n" +
	"--bound\r\n" +
	"Content-Type: text/html; charset=utf-8\r\n" +
	"\r\n" +
	"<p>HTML part</p>\r\n" +
	"--bound\r\n" +
	"Content-Type: application/pdf\r\n" +
	"Content-Disposition: attachment; filename=\"doc.pdf\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"\r\n" +
	"JVBERi0xLjQK\r\n" +
	"--bound--\r\n"

const qpEmail = "From: qp@example.com\r\n" +
	"To: recv@example.com\r\n" +
	"Subject: QP Test\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"Content-Transfer-Encoding: quoted-printable\r\n" +
	"\r\n" +
	"Hello=20World=21\r\n"

func TestParseRaw_MboxFromLineRejected(t *testing.T) {
	// Postfix pipe flags=F prepends mbox "From " — must not use F in master.cf
	mbox := "From sender@example.com Thu Jun 25 17:00:00 2025\r\n" + plainEmail
	_, err := mail.ParseRaw([]byte(mbox), "recv@example.com")
	if err == nil {
		t.Fatal("expected error for mbox-prefixed message")
	}
}

func TestParseRaw_Plain(t *testing.T) {
	e, err := mail.ParseRaw([]byte(plainEmail), "recv@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if e.MailFrom != "sender@example.com" {
		t.Fatalf("from %q", e.MailFrom)
	}
	if e.RcptTo != "recv@example.com" {
		t.Fatalf("rcpt %q", e.RcptTo)
	}
	if e.Subject != "Hello World" {
		t.Fatalf("subject %q", e.Subject)
	}
	if !strings.Contains(e.TextBody, "Plain body") {
		t.Fatalf("text body %q", e.TextBody)
	}
	if e.MessageID != "<test123@example.com>" && e.MessageID != "test123@example.com" {
		t.Fatalf("message-id %q", e.MessageID)
	}
}

func TestParseRaw_Multipart(t *testing.T) {
	e, err := mail.ParseRaw([]byte(multipartEmail), "recv@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.TextBody, "Text part") {
		t.Fatalf("text %q", e.TextBody)
	}
	if !strings.Contains(e.HTMLBody, "HTML part") {
		t.Fatalf("html %q", e.HTMLBody)
	}
	if len(e.Attachments) != 1 {
		t.Fatalf("attachments %d", len(e.Attachments))
	}
	if e.Attachments[0].Filename != "doc.pdf" {
		t.Fatalf("filename %q", e.Attachments[0].Filename)
	}
	if e.Attachments[0].Size <= 0 {
		t.Fatal("expected attachment size > 0")
	}
}

func TestParseRaw_QuotedPrintable(t *testing.T) {
	e, err := mail.ParseRaw([]byte(qpEmail), "recv@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.TextBody, "Hello World!") && !strings.Contains(e.TextBody, "Hello") {
		t.Fatalf("decoded body %q", e.TextBody)
	}
}
