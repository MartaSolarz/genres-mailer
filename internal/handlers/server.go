package handlers

import (
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/msolarzwebsensa/genres-mailer/internal/auth"
	"github.com/msolarzwebsensa/genres-mailer/internal/config"
	"github.com/msolarzwebsensa/genres-mailer/internal/store"
	"github.com/msolarzwebsensa/genres-mailer/web"
	"golang.org/x/crypto/bcrypt"
)

type Server struct {
	cfg       *config.Config
	store     *store.Store
	sessions  *auth.SessionStore
	limiter   *auth.RateLimiter
	logger    *slog.Logger
	tmpl      *template.Template
	staticFS  fs.FS
	dummyHash []byte
}

func NewServer(cfg *config.Config, st *store.Store, sessions *auth.SessionStore, limiter *auth.RateLimiter, logger *slog.Logger) (*Server, error) {
	tmpl, err := template.ParseFS(web.TemplatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parsowanie szablonów: %w", err)
	}

	staticFS, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("podkatalog static: %w", err)
	}

	dummy, err := bcrypt.GenerateFromPassword([]byte("nieistniejące-konto-placeholder"), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("generowanie hasła zastępczego: %w", err)
	}

	return &Server{
		cfg:       cfg,
		store:     st,
		sessions:  sessions,
		limiter:   limiter,
		logger:    logger,
		tmpl:      tmpl,
		staticFS:  staticFS,
		dummyHash: dummy,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", s.handleLoginForm)
	mux.HandleFunc("POST /login", s.handleLoginSubmit)
	mux.HandleFunc("POST /logout", s.handleLogout)
	mux.HandleFunc("GET /{$}", s.handleIndex)

	appChain := s.sessionMiddleware(s.csrfMiddleware(s.authMiddleware(mux)))

	root := http.NewServeMux()
	root.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(s.staticFS))))
	root.HandleFunc("GET /healthz", s.handleHealth)
	root.Handle("/", appChain)

	var h http.Handler = root

	h = SecurityHeaders(h)
	h = RequestLogger(s.logger, h)

	return h
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())

	s.render(w, r, "panel.html", map[string]any{
		"Username":  sess.Username,
		"CSRFToken": sess.CSRFToken,
	})
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		s.logger.Error("renderowanie szablonu nie powiodło się",
			"request_id", RequestID(r.Context()), "template", name, "error", err)
	}
}
