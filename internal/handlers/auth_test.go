package handlers_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/msolarzwebsensa/genres-mailer/internal/auth"
	"github.com/msolarzwebsensa/genres-mailer/internal/config"
	"github.com/msolarzwebsensa/genres-mailer/internal/handlers"
	"github.com/msolarzwebsensa/genres-mailer/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const (
	testUser = "genetyk"
	testPass = "haslo-testowe-123"
)

var csrfRe = regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)

func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()

	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	t.Cleanup(func() { _ = st.Close() })

	hash, err := bcrypt.GenerateFromPassword([]byte(testPass), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}

	if err := st.CreateUser(context.Background(), testUser, string(hash)); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	cfg := &config.Config{ListenAddr: "127.0.0.1:0"}
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

func newClient(t *testing.T) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}

	return &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func fetchLoginCSRF(t *testing.T, client *http.Client, base string) string {
	t.Helper()

	resp, err := client.Get(base + "/login")
	if err != nil {
		t.Fatalf("GET /login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	m := csrfRe.FindSubmatch(body)
	if m == nil {
		t.Fatal("nie znaleziono tokenu CSRF w formularzu logowania")
	}

	return string(m[1])
}

func postLogin(client *http.Client, base, csrf, user, pass string) (*http.Response, error) {
	form := url.Values{}
	form.Set("csrf_token", csrf)
	form.Set("username", user)
	form.Set("password", pass)

	req, err := http.NewRequest(http.MethodPost, base+"/login", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return client.Do(req)
}

func TestLoginSuccessRedirects(t *testing.T) {
	ts, _ := newTestServer(t)
	client := newClient(t)

	csrf := fetchLoginCSRF(t, client, ts.URL)

	resp, err := postLogin(client, ts.URL, csrf, testUser, testPass)
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("oczekiwano 303, otrzymano %d", resp.StatusCode)
	}

	if loc := resp.Header.Get("Location"); loc != "/" {
		t.Fatalf("oczekiwano przekierowania na /, otrzymano %q", loc)
	}
}

func TestLoginWrongPasswordUnauthorized(t *testing.T) {
	ts, _ := newTestServer(t)
	client := newClient(t)

	csrf := fetchLoginCSRF(t, client, ts.URL)

	resp, err := postLogin(client, ts.URL, csrf, testUser, "zle-haslo")
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("oczekiwano 401, otrzymano %d", resp.StatusCode)
	}
}

func TestLoginWithoutCSRFForbidden(t *testing.T) {
	ts, _ := newTestServer(t)
	client := newClient(t)

	fetchLoginCSRF(t, client, ts.URL)

	resp, err := postLogin(client, ts.URL, "", testUser, testPass)
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("oczekiwano 403 dla braku tokenu CSRF, otrzymano %d", resp.StatusCode)
	}
}

func TestLoginRateLimited(t *testing.T) {
	ts, _ := newTestServer(t)
	client := newClient(t)

	csrf := fetchLoginCSRF(t, client, ts.URL)

	for range 5 {
		resp, err := postLogin(client, ts.URL, csrf, testUser, "zle-haslo")
		if err != nil {
			t.Fatalf("POST /login: %v", err)
		}

		_ = resp.Body.Close()
	}

	resp, err := postLogin(client, ts.URL, csrf, testUser, testPass)
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Zbyt wiele prób") {
		t.Fatal("po przekroczeniu limitu powinien pojawić się komunikat o zbyt wielu próbach")
	}
}

func TestProtectedRedirectsWhenAnonymous(t *testing.T) {
	ts, _ := newTestServer(t)
	client := newClient(t)

	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
		t.Fatalf("anonimowy dostęp do / powinien przekierować na /login, otrzymano %d %q",
			resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestLogoutClearsSession(t *testing.T) {
	ts, _ := newTestServer(t)
	client := newClient(t)

	csrf := fetchLoginCSRF(t, client, ts.URL)

	resp, err := postLogin(client, ts.URL, csrf, testUser, testPass)
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}

	_ = resp.Body.Close()

	panelResp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}

	panelBody, _ := io.ReadAll(panelResp.Body)
	_ = panelResp.Body.Close()

	m := csrfRe.FindSubmatch(panelBody)
	if m == nil {
		t.Fatal("panel powinien zawierać token CSRF do wylogowania")
	}

	form := url.Values{}
	form.Set("csrf_token", string(m[1]))

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/logout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	logoutResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /logout: %v", err)
	}

	_ = logoutResp.Body.Close()

	afterResp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / po wylogowaniu: %v", err)
	}
	defer func() { _ = afterResp.Body.Close() }()

	if afterResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("po wylogowaniu dostęp do / powinien przekierować, otrzymano %d", afterResp.StatusCode)
	}
}
