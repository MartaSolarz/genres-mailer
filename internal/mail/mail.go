package mail

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"strconv"
	"time"
)

type Config struct {
	Host    string
	Port    int
	User    string
	Pass    string
	From    string
	OrgName string
	Timeout time.Duration
}

type Sender struct {
	cfg Config
}

func NewSender(cfg Config) *Sender {
	return &Sender{cfg: cfg}
}

// SendDocument wysyła wiadomość z załączonym zaszyfrowanym dokumentem PDF.
func (s *Sender) SendDocument(to string, attachment []byte, filename string) error {
	subject := "Dokument z " + s.cfg.OrgName

	body, err := renderDocumentBody(s.cfg.OrgName)
	if err != nil {
		return err
	}

	msg, err := buildMessage(s.cfg.From, s.cfg.OrgName, to, subject, body, &pdfAttachment{name: filename, data: attachment})
	if err != nil {
		return err
	}

	return s.send(to, msg)
}

// SendPassword wysyła neutralną wiadomość z hasłem do dokumentu. Temat i treść
// nie zawierają ID próbki ani słowa „hasło", aby utrudnić automatyczne parowanie.
func (s *Sender) SendPassword(to, password string) error {
	subject := "Informacja uzupełniająca"

	body, err := renderPasswordBody(password)
	if err != nil {
		return err
	}

	msg, err := buildMessage(s.cfg.From, s.cfg.OrgName, to, subject, body, nil)
	if err != nil {
		return err
	}

	return s.send(to, msg)
}

type pdfAttachment struct {
	name string
	data []byte
}

// send nawiązuje połączenie SMTP z limitem czasu i wysyła surową wiadomość.
// Gdy skonfigurowano użytkownika, używa STARTTLS + PlainAuth; w przeciwnym
// razie wysyła przez relay bez uwierzytelnienia (np. lokalny Mailpit).
func (s *Sender) send(to string, msg []byte) error {
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))

	conn, err := net.DialTimeout("tcp", addr, s.cfg.Timeout)
	if err != nil {
		return fmt.Errorf("połączenie SMTP: %w", err)
	}

	if err := conn.SetDeadline(time.Now().Add(s.cfg.Timeout)); err != nil {
		_ = conn.Close()

		return fmt.Errorf("ustawienie limitu czasu SMTP: %w", err)
	}

	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()

		return fmt.Errorf("klient SMTP: %w", err)
	}

	defer func() { _ = c.Close() }()

	if s.cfg.User != "" {
		if ok, _ := c.Extension("STARTTLS"); ok {
			tlsCfg := &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}
			if err := c.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("STARTTLS: %w", err)
			}
		}

		auth := smtp.PlainAuth("", s.cfg.User, s.cfg.Pass, s.cfg.Host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("uwierzytelnienie SMTP: %w", err)
		}
	}

	if err := c.Mail(s.cfg.From); err != nil {
		return fmt.Errorf("polecenie MAIL FROM: %w", err)
	}

	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("polecenie RCPT TO: %w", err)
	}

	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("polecenie DATA: %w", err)
	}

	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("zapis treści wiadomości: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("zamknięcie treści wiadomości: %w", err)
	}

	if err := c.Quit(); err != nil {
		return fmt.Errorf("zakończenie sesji SMTP: %w", err)
	}

	return nil
}

func buildMessage(from, orgName, to, subject string, body messageBody, att *pdfAttachment) ([]byte, error) {
	var buf bytes.Buffer

	fromHeader := fmt.Sprintf("%s <%s>", mime.QEncoding.Encode("utf-8", orgName), from)

	headers := textproto.MIMEHeader{}
	headers.Set("From", fromHeader)
	headers.Set("To", to)
	headers.Set("Subject", mime.QEncoding.Encode("utf-8", subject))
	headers.Set("Date", time.Now().Format(time.RFC1123Z))
	headers.Set("MIME-Version", "1.0")

	mixed := multipart.NewWriter(&buf)

	if att != nil {
		headers.Set("Content-Type", "multipart/mixed; boundary="+mixed.Boundary())
	} else {
		headers.Set("Content-Type", "multipart/alternative; boundary="+mixed.Boundary())
	}

	if err := writeHeaders(&buf, headers); err != nil {
		return nil, err
	}

	if att != nil {
		if err := writeAlternativePart(mixed, body); err != nil {
			return nil, err
		}

		if err := writeAttachment(mixed, att); err != nil {
			return nil, err
		}
	} else {
		if err := writeAlternativeInline(mixed, body); err != nil {
			return nil, err
		}
	}

	if err := mixed.Close(); err != nil {
		return nil, fmt.Errorf("zamknięcie wiadomości: %w", err)
	}

	return buf.Bytes(), nil
}

// writeAlternativePart osadza multipart/alternative jako część nadrzędnego
// multipart/mixed (używane przy załączniku).
func writeAlternativePart(mixed *multipart.Writer, body messageBody) error {
	var altBuf bytes.Buffer

	alt := multipart.NewWriter(&altBuf)

	if err := writeBodyParts(alt, body); err != nil {
		return err
	}

	if err := alt.Close(); err != nil {
		return fmt.Errorf("zamknięcie części alternatywnej: %w", err)
	}

	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Type", "multipart/alternative; boundary="+alt.Boundary())

	part, err := mixed.CreatePart(partHeader)
	if err != nil {
		return fmt.Errorf("część alternatywna: %w", err)
	}

	if _, err := part.Write(altBuf.Bytes()); err != nil {
		return fmt.Errorf("zapis części alternatywnej: %w", err)
	}

	return nil
}

// writeAlternativeInline zapisuje części tekstową i HTML bezpośrednio do
// nadrzędnego multipart/alternative (używane bez załącznika).
func writeAlternativeInline(alt *multipart.Writer, body messageBody) error {
	return writeBodyParts(alt, body)
}

func writeBodyParts(alt *multipart.Writer, body messageBody) error {
	textHeader := textproto.MIMEHeader{}
	textHeader.Set("Content-Type", "text/plain; charset=utf-8")

	tp, err := alt.CreatePart(textHeader)
	if err != nil {
		return fmt.Errorf("część tekstowa: %w", err)
	}

	if _, err := tp.Write([]byte(body.Text)); err != nil {
		return fmt.Errorf("zapis części tekstowej: %w", err)
	}

	htmlHeader := textproto.MIMEHeader{}
	htmlHeader.Set("Content-Type", "text/html; charset=utf-8")

	hp, err := alt.CreatePart(htmlHeader)
	if err != nil {
		return fmt.Errorf("część HTML: %w", err)
	}

	if _, err := hp.Write([]byte(body.HTML)); err != nil {
		return fmt.Errorf("zapis części HTML: %w", err)
	}

	return nil
}

func writeAttachment(mixed *multipart.Writer, att *pdfAttachment) error {
	header := textproto.MIMEHeader{}
	header.Set("Content-Type", "application/pdf")
	header.Set("Content-Transfer-Encoding", "base64")
	header.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, att.name))

	part, err := mixed.CreatePart(header)
	if err != nil {
		return fmt.Errorf("załącznik: %w", err)
	}

	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(att.data)))
	base64.StdEncoding.Encode(encoded, att.data)

	// Łamanie linii co 76 znaków zgodnie z MIME.
	for len(encoded) > 76 {
		if _, err := part.Write(encoded[:76]); err != nil {
			return fmt.Errorf("zapis załącznika: %w", err)
		}

		if _, err := part.Write([]byte("\r\n")); err != nil {
			return fmt.Errorf("zapis załącznika: %w", err)
		}

		encoded = encoded[76:]
	}

	if _, err := part.Write(encoded); err != nil {
		return fmt.Errorf("zapis załącznika: %w", err)
	}

	return nil
}

func writeHeaders(buf *bytes.Buffer, headers textproto.MIMEHeader) error {
	for _, k := range []string{"From", "To", "Subject", "Date", "MIME-Version", "Content-Type"} {
		if v := headers.Get(k); v != "" {
			if _, err := fmt.Fprintf(buf, "%s: %s\r\n", k, v); err != nil {
				return fmt.Errorf("zapis nagłówka: %w", err)
			}
		}
	}

	if _, err := buf.WriteString("\r\n"); err != nil {
		return fmt.Errorf("zapis separatora nagłówków: %w", err)
	}

	return nil
}
