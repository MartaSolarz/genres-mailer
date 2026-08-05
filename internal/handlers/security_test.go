package handlers_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	ts, _, _, _ := newJobServer(t)

	resp, err := http.Get(ts.URL + "/login")
	if err != nil {
		t.Fatalf("GET /login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	checks := map[string]string{
		"Content-Security-Policy": "default-src 'self'",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
		"Cache-Control":           "no-store",
	}

	for header, want := range checks {
		if got := resp.Header.Get(header); !strings.Contains(got, want) {
			t.Errorf("%s = %q, oczekiwano zawierające %q", header, got, want)
		}
	}
}

func TestPreviewHeaderSameOrigin(t *testing.T) {
	ts, _, _, _ := newJobServer(t)
	client, csrf := loginClient(t, ts.URL, "userA")

	resp, err := uploadPDF(t, client, ts.URL, csrf, "PROBKA-001", minimalPDF)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	jobUUID, _ := decodeJSON(t, resp)["job_uuid"].(string)

	prev, err := client.Get(ts.URL + "/api/jobs/" + jobUUID + "/preview")
	if err != nil {
		t.Fatalf("GET preview: %v", err)
	}
	defer func() { _ = prev.Body.Close() }()

	if xfo := prev.Header.Get("X-Frame-Options"); xfo != "SAMEORIGIN" {
		t.Fatalf("preview X-Frame-Options = %q, oczekiwano SAMEORIGIN", xfo)
	}
}

func TestSessionCookieHardening(t *testing.T) {
	ts, _, _, _ := newJobServer(t)
	client := newClient(t)

	csrf := fetchLoginCSRF(t, client, ts.URL)

	resp, err := postLogin(client, ts.URL, csrf, "userA", testPass)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var cookie string

	for _, c := range resp.Header["Set-Cookie"] {
		if strings.HasPrefix(c, sessionCookiePrefix) {
			cookie = c
		}
	}

	if cookie == "" {
		t.Fatal("brak cookie sesji po zalogowaniu")
	}

	for _, attr := range []string{"HttpOnly", "SameSite=Strict", "Path=/"} {
		if !strings.Contains(cookie, attr) {
			t.Errorf("cookie sesji nie zawiera %q: %s", attr, cookie)
		}
	}
}

func TestUploadRemovesTempFile(t *testing.T) {
	ts, _, _, dir := newJobServer(t)
	client, csrf := loginClient(t, ts.URL, "userA")

	resp, err := uploadPDF(t, client, ts.URL, csrf, "PROBKA-001", minimalPDF)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	_ = resp.Body.Close()

	tmpEntries, err := os.ReadDir(filepath.Join(dir, "tmp"))
	if err != nil {
		t.Fatalf("odczyt katalogu tmp: %v", err)
	}

	if len(tmpEntries) != 0 {
		t.Fatalf("plik tymczasowy nie został usunięty (%d plików w tmp)", len(tmpEntries))
	}

	encEntries, err := os.ReadDir(filepath.Join(dir, "encrypted"))
	if err != nil {
		t.Fatalf("odczyt katalogu encrypted: %v", err)
	}

	if len(encEntries) != 1 {
		t.Fatalf("oczekiwano 1 zaszyfrowanego pliku, jest %d", len(encEntries))
	}
}

const sessionCookiePrefix = "session="
