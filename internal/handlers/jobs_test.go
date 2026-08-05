package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/msolarzwebsensa/genres-mailer/internal/auth"
	"github.com/msolarzwebsensa/genres-mailer/internal/config"
	"github.com/msolarzwebsensa/genres-mailer/internal/handlers"
	"github.com/msolarzwebsensa/genres-mailer/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const minimalPDF = "%PDF-1.4\n" +
	"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n" +
	"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n" +
	"3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]>>endobj\n" +
	"trailer<</Size 4/Root 1 0 R>>\n%%EOF\n"

func newJobServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()

	dir := t.TempDir()
	for _, sub := range []string{"tmp", "encrypted"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	t.Cleanup(func() { _ = st.Close() })

	ctx := context.Background()
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPass), bcrypt.MinCost)

	for _, u := range []string{"userA", "userB"} {
		if err := st.CreateUser(ctx, u, string(hash)); err != nil {
			t.Fatalf("CreateUser %s: %v", u, err)
		}
	}

	if _, err := st.UpsertSample(ctx, "PROBKA-001", "jan.kowalski@example.org"); err != nil {
		t.Fatalf("UpsertSample: %v", err)
	}

	cfg := &config.Config{
		DataDir:       dir,
		AppSecretKey:  bytes.Repeat([]byte("k"), 32),
		MaxUploadMB:   25,
		MaxJobsPerDay: 200,
		FileRetention: 72 * time.Hour,
	}
	sessions := auth.NewSessionStore(time.Hour)
	limiter := auth.NewRateLimiter(5, 15*time.Minute)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	srv, err := handlers.NewServer(cfg, st, sessions, limiter, logger)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return ts, st
}

func loginClient(t *testing.T, base, user string) (*http.Client, string) {
	t.Helper()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	csrf := fetchLoginCSRF(t, client, base)

	resp, err := postLogin(client, base, csrf, user, testPass)
	if err != nil {
		t.Fatalf("login %s: %v", user, err)
	}

	_ = resp.Body.Close()

	panelResp, err := client.Get(base + "/")
	if err != nil {
		t.Fatalf("GET panel: %v", err)
	}

	body, _ := io.ReadAll(panelResp.Body)
	_ = panelResp.Body.Close()

	m := csrfRe.FindSubmatch(body)
	if m == nil {
		t.Fatal("nie znaleziono tokenu CSRF w panelu")
	}

	return client, string(m[1])
}

func uploadPDF(t *testing.T, client *http.Client, base, csrf, sampleID, content string) (*http.Response, error) {
	t.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("sample_id", sampleID)

	fw, err := mw.CreateFormFile("file", "wyniki.pdf")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}

	_, _ = fw.Write([]byte(content))
	_ = mw.Close()

	req, err := http.NewRequest(http.MethodPost, base+"/api/jobs", &buf)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", csrf)

	return client.Do(req)
}

func decodeJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()

	var out map[string]any
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("JSON %q: %v", string(body), err)
	}

	return out
}

func TestJobFullFlow(t *testing.T) {
	ts, _ := newJobServer(t)
	client, csrf := loginClient(t, ts.URL, "userA")

	resp, err := uploadPDF(t, client, ts.URL, csrf, "PROBKA-001", minimalPDF)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload oczekiwano 200, otrzymano %d", resp.StatusCode)
	}

	data := decodeJSON(t, resp)
	jobUUID, _ := data["job_uuid"].(string)

	if jobUUID == "" {
		t.Fatal("brak job_uuid w odpowiedzi")
	}

	if masked, _ := data["recipient_masked"].(string); masked != "j***@example.org" {
		t.Fatalf("maskowanie e-mail = %q", masked)
	}

	pwResp, err := client.Get(ts.URL + "/api/jobs/" + jobUUID + "/password")
	if err != nil {
		t.Fatalf("GET password: %v", err)
	}

	if pwResp.StatusCode != http.StatusOK {
		t.Fatalf("password oczekiwano 200, otrzymano %d", pwResp.StatusCode)
	}

	pwData := decodeJSON(t, pwResp)
	if pw, _ := pwData["password"].(string); len(pw) != 16 {
		t.Fatalf("hasło powinno mieć 16 znaków, ma %d", len(pw))
	}

	prevResp, err := client.Get(ts.URL + "/api/jobs/" + jobUUID + "/preview")
	if err != nil {
		t.Fatalf("GET preview: %v", err)
	}
	defer func() { _ = prevResp.Body.Close() }()

	if prevResp.StatusCode != http.StatusOK {
		t.Fatalf("preview oczekiwano 200, otrzymano %d", prevResp.StatusCode)
	}

	if ct := prevResp.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("preview Content-Type = %q", ct)
	}
}

func TestJobSharedAccess(t *testing.T) {
	ts, _ := newJobServer(t)

	clientA, csrfA := loginClient(t, ts.URL, "userA")

	resp, err := uploadPDF(t, clientA, ts.URL, csrfA, "PROBKA-001", minimalPDF)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	data := decodeJSON(t, resp)
	jobUUID, _ := data["job_uuid"].(string)

	clientB, _ := loginClient(t, ts.URL, "userB")

	for _, path := range []string{"/preview", "/download", "/password"} {
		r, err := clientB.Get(ts.URL + "/api/jobs/" + jobUUID + path)
		if err != nil {
			t.Fatalf("GET %s jako userB: %v", path, err)
		}

		_ = r.Body.Close()

		if r.StatusCode != http.StatusOK {
			t.Fatalf("dostęp współdzielony do %s: oczekiwano 200, otrzymano %d", path, r.StatusCode)
		}
	}
}

func TestJobDelete(t *testing.T) {
	ts, _ := newJobServer(t)

	clientA, csrfA := loginClient(t, ts.URL, "userA")

	resp, err := uploadPDF(t, clientA, ts.URL, csrfA, "PROBKA-001", minimalPDF)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	data := decodeJSON(t, resp)
	jobUUID, _ := data["job_uuid"].(string)

	clientB, csrfB := loginClient(t, ts.URL, "userB")

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/jobs/"+jobUUID, nil)
	req.Header.Set("X-CSRF-Token", csrfB)

	delResp, err := clientB.Do(req)
	if err != nil {
		t.Fatalf("DELETE jako userB: %v", err)
	}

	_ = delResp.Body.Close()

	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("usunięcie przez innego użytkownika: oczekiwano 200, otrzymano %d", delResp.StatusCode)
	}

	after, err := clientA.Get(ts.URL + "/api/jobs/" + jobUUID + "/password")
	if err != nil {
		t.Fatalf("GET po usunięciu: %v", err)
	}

	_ = after.Body.Close()

	if after.StatusCode != http.StatusNotFound {
		t.Fatalf("po usunięciu job powinien zwracać 404, otrzymano %d", after.StatusCode)
	}
}

func TestJobDeleteRequiresCSRF(t *testing.T) {
	ts, _ := newJobServer(t)

	clientA, csrfA := loginClient(t, ts.URL, "userA")

	resp, err := uploadPDF(t, clientA, ts.URL, csrfA, "PROBKA-001", minimalPDF)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	data := decodeJSON(t, resp)
	jobUUID, _ := data["job_uuid"].(string)

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/jobs/"+jobUUID, nil)

	delResp, err := clientA.Do(req)
	if err != nil {
		t.Fatalf("DELETE bez CSRF: %v", err)
	}

	_ = delResp.Body.Close()

	if delResp.StatusCode != http.StatusForbidden {
		t.Fatalf("usunięcie bez CSRF: oczekiwano 403, otrzymano %d", delResp.StatusCode)
	}
}

func TestJobRejectsNonPDF(t *testing.T) {
	ts, _ := newJobServer(t)
	client, csrf := loginClient(t, ts.URL, "userA")

	resp, err := uploadPDF(t, client, ts.URL, csrf, "PROBKA-001", "to nie jest PDF")
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("plik nie-PDF: oczekiwano 400, otrzymano %d", resp.StatusCode)
	}
}

func TestJobUploadRequiresCSRF(t *testing.T) {
	ts, _ := newJobServer(t)
	client, _ := loginClient(t, ts.URL, "userA")

	resp, err := uploadPDF(t, client, ts.URL, "", "PROBKA-001", minimalPDF)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("upload bez CSRF: oczekiwano 403, otrzymano %d", resp.StatusCode)
	}
}

func TestJobSentIsReadOnly(t *testing.T) {
	ts, st := newJobServer(t)
	client, csrf := loginClient(t, ts.URL, "userA")

	resp, err := uploadPDF(t, client, ts.URL, csrf, "PROBKA-001", minimalPDF)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	data := decodeJSON(t, resp)
	jobUUID, _ := data["job_uuid"].(string)

	if err := st.MarkJobSent(context.Background(), jobUUID, time.Now()); err != nil {
		t.Fatalf("MarkJobSent: %v", err)
	}

	for _, path := range []string{"/preview", "/download", "/password"} {
		r, err := client.Get(ts.URL + "/api/jobs/" + jobUUID + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}

		_ = r.Body.Close()

		if r.StatusCode != http.StatusForbidden {
			t.Fatalf("wysłany dokument %s: oczekiwano 403, otrzymano %d", path, r.StatusCode)
		}
	}

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/jobs/"+jobUUID, nil)
	req.Header.Set("X-CSRF-Token", csrf)

	delResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}

	_ = delResp.Body.Close()

	if delResp.StatusCode != http.StatusConflict {
		t.Fatalf("usunięcie wysłanego: oczekiwano 409, otrzymano %d", delResp.StatusCode)
	}
}
