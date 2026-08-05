package mail

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func fakeSMTP(t *testing.T) (host string, port int, received <-chan string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ch := make(chan string, 1)

	go func() {
		conn, aerr := ln.Accept()
		if aerr != nil {
			return
		}

		defer func() { _ = conn.Close() }()
		defer func() { _ = ln.Close() }()

		r := bufio.NewReader(conn)
		writeLine(conn, "220 mock ESMTP")

		var body strings.Builder

		inData := false

		for {
			line, rerr := r.ReadString('\n')
			if rerr != nil {
				return
			}

			trimmed := strings.TrimRight(line, "\r\n")

			if inData {
				if trimmed == "." {
					inData = false

					writeLine(conn, "250 ok")
					ch <- body.String()

					continue
				}

				body.WriteString(line)

				continue
			}

			switch {
			case strings.HasPrefix(trimmed, "EHLO"), strings.HasPrefix(trimmed, "HELO"):
				writeLine(conn, "250 mock")
			case strings.HasPrefix(trimmed, "MAIL"), strings.HasPrefix(trimmed, "RCPT"):
				writeLine(conn, "250 ok")
			case strings.HasPrefix(trimmed, "DATA"):
				inData = true

				writeLine(conn, "354 end with .")
			case strings.HasPrefix(trimmed, "QUIT"):
				writeLine(conn, "221 bye")

				return
			default:
				writeLine(conn, "250 ok")
			}
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)

	return "127.0.0.1", addr.Port, ch
}

func writeLine(conn net.Conn, s string) {
	_, _ = fmt.Fprintf(conn, "%s\r\n", s)
}

func newTestSender(host string, port int) *Sender {
	return NewSender(Config{
		Host:    host,
		Port:    port,
		From:    "wyniki@example.org",
		OrgName: "Pracownia testowa",
		Timeout: 5 * time.Second,
	})
}

func TestSendDocumentHasAttachment(t *testing.T) {
	host, port, ch := fakeSMTP(t)
	s := newTestSender(host, port)

	if err := s.SendDocument("odbiorca@example.org", []byte("%PDF-1.4 dane"), "dokument.pdf"); err != nil {
		t.Fatalf("SendDocument: %v", err)
	}

	msg := waitMsg(t, ch)

	if !strings.Contains(msg, "multipart/mixed") {
		t.Error("wiadomość z załącznikiem powinna być multipart/mixed")
	}

	if !strings.Contains(msg, `filename="dokument.pdf"`) {
		t.Error("brak nazwy załącznika")
	}

	if !strings.Contains(msg, "application/pdf") {
		t.Error("brak typu application/pdf")
	}

	if !strings.Contains(msg, "Subject:") {
		t.Error("brak nagłówka Subject")
	}
}

func TestSendPasswordContainsPassword(t *testing.T) {
	host, port, ch := fakeSMTP(t)
	s := newTestSender(host, port)

	if err := s.SendPassword("odbiorca@example.org", "Xk7mR2ph9Tn4Qd58"); err != nil {
		t.Fatalf("SendPassword: %v", err)
	}

	msg := waitMsg(t, ch)

	if !strings.Contains(msg, "Xk7mR2ph9Tn4Qd58") {
		t.Error("wiadomość powinna zawierać hasło")
	}

	if strings.Contains(strings.ToLower(msg), "próbk") {
		t.Error("wiadomość z hasłem nie powinna zawierać informacji o próbce")
	}
}

func waitMsg(t *testing.T, ch <-chan string) string {
	t.Helper()

	select {
	case msg := <-ch:
		return msg
	case <-time.After(3 * time.Second):
		t.Fatal("nie otrzymano wiadomości w oczekiwanym czasie")

		return ""
	}
}
