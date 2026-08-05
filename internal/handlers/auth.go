package handlers

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"github.com/msolarzwebsensa/genres-mailer/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName = "session"
	maxFormBytes      = 64 << 10
)

type sessionCtxKey struct{}

func SessionFromContext(ctx context.Context) *auth.Session {
	if v, ok := ctx.Value(sessionCtxKey{}).(*auth.Session); ok {
		return v
	}

	return nil
}

func (s *Server) sessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil {
			if sess, ok := s.sessions.Get(cookie.Value); ok {
				ctx := context.WithValue(r.Context(), sessionCtxKey{}, sess)
				next.ServeHTTP(w, r.WithContext(ctx))

				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSafeMethod(r.Method) {
			next.ServeHTTP(w, r)

			return
		}

		token := r.Header.Get("X-CSRF-Token")
		if token == "" && isFormContentType(r) {
			r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)

			if err := r.ParseForm(); err != nil {
				http.Error(w, "Nieprawidłowe dane formularza", http.StatusBadRequest)

				return
			}

			token = r.PostForm.Get("csrf_token")
		}

		sess := SessionFromContext(r.Context())
		if sess == nil || !validCSRFToken(token, sess.CSRFToken) {
			http.Error(w, "Nieprawidłowy token CSRF", http.StatusForbidden)

			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			next.ServeHTTP(w, r)

			return
		}

		sess := SessionFromContext(r.Context())
		if sess == nil || !sess.Authenticated() {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, "Wymagane uwierzytelnienie", http.StatusUnauthorized)

				return
			}

			http.Redirect(w, r, "/login", http.StatusSeeOther)

			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	if sess != nil && sess.Authenticated() {
		http.Redirect(w, r, "/", http.StatusSeeOther)

		return
	}

	if sess == nil {
		newSess, err := s.sessions.New()
		if err != nil {
			http.Error(w, "Błąd serwera", http.StatusInternalServerError)

			return
		}

		s.setSessionCookie(w, r, newSess.Token)
		sess = newSess
	}

	s.render(w, r, "login.html", map[string]any{"CSRFToken": sess.CSRFToken})
}

func (s *Server) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())

	username := strings.TrimSpace(r.PostForm.Get("username"))
	password := r.PostForm.Get("password")
	key := clientIP(r) + "|" + strings.ToLower(username)

	if !s.limiter.Allowed(key) {
		s.logger.Warn("przekroczono limit prób logowania",
			"request_id", RequestID(r.Context()), "username", username)
		s.renderLoginError(w, r, sess.CSRFToken, "Zbyt wiele prób logowania. Spróbuj ponownie za kilka minut.")

		return
	}

	userID, ok := s.verifyCredentials(r, username, password)
	if !ok {
		s.limiter.RecordFailure(key)
		s.logger.Warn("nieudane logowanie",
			"request_id", RequestID(r.Context()), "username", username)
		s.renderLoginError(w, r, sess.CSRFToken, "Nieprawidłowa nazwa użytkownika lub hasło.")

		return
	}

	s.limiter.Reset(key)

	newSess, err := s.sessions.Authenticate(sess.Token, userID, username)
	if err != nil {
		http.Error(w, "Błąd serwera", http.StatusInternalServerError)

		return
	}

	s.setSessionCookie(w, r, newSess.Token)

	if err := s.store.InsertAudit(r.Context(), &userID, "login", "", ""); err != nil {
		s.logger.Error("zapis audytu logowania nie powiódł się",
			"request_id", RequestID(r.Context()), "error", err)
	}

	s.logger.Info("udane logowanie",
		"request_id", RequestID(r.Context()), "user", username)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	if sess != nil {
		s.sessions.Delete(sess.Token)
	}

	s.clearSessionCookie(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) verifyCredentials(r *http.Request, username, password string) (int64, bool) {
	user, err := s.store.GetUserByUsername(r.Context(), username)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))

		return 0, false
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return 0, false
	}

	if user.Disabled {
		return 0, false
	}

	return user.ID, true
}

func (s *Server) renderLoginError(w http.ResponseWriter, r *http.Request, csrf, msg string) {
	w.WriteHeader(http.StatusUnauthorized)
	s.render(w, r, "login.html", map[string]any{"CSRFToken": csrf, "Error": msg})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(s.cfg.SessionTTL().Seconds()),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureRequest(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func validCSRFToken(got, want string) bool {
	if got == "" || want == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func isFormContentType(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")

	return strings.HasPrefix(ct, "application/x-www-form-urlencoded")
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func isSecureRequest(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")

		return strings.TrimSpace(parts[0])
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}
