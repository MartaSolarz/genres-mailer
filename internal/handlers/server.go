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
	mux.HandleFunc("POST /api/jobs", s.handleCreateJob)
	mux.HandleFunc("GET /api/jobs/{uuid}/preview", s.handlePreview)
	mux.HandleFunc("GET /api/jobs/{uuid}/download", s.handleDownload)
	mux.HandleFunc("GET /api/jobs/{uuid}/password", s.handlePassword)
	mux.HandleFunc("DELETE /api/jobs/{uuid}", s.handleDeleteJob)

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

type sampleView struct {
	SampleID string
	Masked   string
}

type jobView struct {
	UUID       string
	SampleID   string
	Recipient  string
	Username   string
	Status     string
	StatusRaw  string
	Created    string
	Actionable bool
	CanDelete  bool
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	reqID := RequestID(r.Context())

	samples, err := s.store.ListSamples(r.Context())
	if err != nil {
		s.serverError(w, reqID, "lista próbek", err)

		return
	}

	views := make([]sampleView, 0, len(samples))
	maskBySample := make(map[string]string, len(samples))

	for _, sm := range samples {
		masked := maskEmail(sm.RecipientEmail)
		views = append(views, sampleView{SampleID: sm.SampleID, Masked: masked})
		maskBySample[sm.SampleID] = masked
	}

	jobs, err := s.store.ListJobs(r.Context(), 50)
	if err != nil {
		s.serverError(w, reqID, "historia jobów", err)

		return
	}

	history := make([]jobView, 0, len(jobs))
	for _, j := range jobs {
		recipient := maskBySample[j.SampleID]
		if recipient == "" {
			recipient = "—"
		}

		history = append(history, jobView{
			UUID:       j.UUID,
			SampleID:   j.SampleID,
			Recipient:  recipient,
			Username:   j.Username,
			Status:     statusLabel(j.Status),
			StatusRaw:  j.Status,
			Created:    j.CreatedAt.Local().Format("2006-01-02 15:04"),
			Actionable: j.Status == "encrypted",
			CanDelete:  j.Status != "sent",
		})
	}

	s.render(w, r, "panel.html", map[string]any{
		"Username":  sess.Username,
		"CSRFToken": sess.CSRFToken,
		"Samples":   views,
		"History":   history,
	})
}

func statusLabel(status string) string {
	switch status {
	case "encrypted":
		return "Zaszyfrowany"
	case "sent":
		return "Wysłany"
	case "expired":
		return "Wygasły"
	default:
		return status
	}
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		s.logger.Error("renderowanie szablonu nie powiodło się",
			"request_id", RequestID(r.Context()), "template", name, "error", err)
	}
}
