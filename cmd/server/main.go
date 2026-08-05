package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/msolarzwebsensa/genres-mailer/internal/auth"
	"github.com/msolarzwebsensa/genres-mailer/internal/config"
	"github.com/msolarzwebsensa/genres-mailer/internal/handlers"
	"github.com/msolarzwebsensa/genres-mailer/internal/mail"
	"github.com/msolarzwebsensa/genres-mailer/internal/store"
)

func main() {
	syscall.Umask(0o077)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("start aplikacji nie powiódł się", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("wczytanie konfiguracji: %w", err)
	}

	for _, dir := range []string{cfg.DataDir, cfg.DataDir + "/tmp", cfg.DataDir + "/encrypted"} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("utworzenie katalogu %s: %w", dir, err)
		}
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("inicjalizacja bazy danych: %w", err)
	}

	defer func() { _ = st.Close() }()

	logger.Info("baza otwarta, migracje zastosowane", "db_path", cfg.DBPath)

	sessions := auth.NewSessionStore(cfg.SessionTTL())
	limiter := auth.NewRateLimiter(5, 15*time.Minute)

	sender := mail.NewSender(mail.Config{
		Host:    cfg.SMTPHost,
		Port:    cfg.SMTPPort,
		User:    cfg.SMTPUser,
		Pass:    cfg.SMTPPass,
		From:    cfg.SMTPFrom,
		OrgName: cfg.OrgName,
		Timeout: cfg.SMTPTimeout(),
	})

	srvHandlers, err := handlers.NewServer(cfg, st, sessions, limiter, sender, logger)
	if err != nil {
		return fmt.Errorf("inicjalizacja serwera HTTP: %w", err)
	}

	stopGC := make(chan struct{})
	go runSessionGC(sessions, stopGC)

	defer close(stopGC)

	stopCleanup := make(chan struct{})
	go runCleanup(st, logger, cfg.DataDir, stopCleanup)

	defer close(stopCleanup)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srvHandlers.Handler(),
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	serverErr := make(chan error, 1)

	go func() {
		logger.Info("serwer nasłuchuje", "addr", cfg.ListenAddr)

		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return err
	case sig := <-stop:
		logger.Info("otrzymano sygnał, zamykanie serwera", "signal", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("zamykanie serwera: %w", err)
	}

	logger.Info("serwer zamknięty poprawnie")

	return nil
}

func runSessionGC(sessions *auth.SessionStore, stop <-chan struct{}) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sessions.GC()
		case <-stop:
			return
		}
	}
}

func runCleanup(st *store.Store, logger *slog.Logger, dataDir string, stop <-chan struct{}) {
	cleanupExpired(st, logger, dataDir)

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cleanupExpired(st, logger, dataDir)
		case <-stop:
			return
		}
	}
}

// cleanupExpired usuwa pliki i zeruje hasła jobów po terminie retencji.
// Dokumenty wysłane zachowują status "sent" (są tylko wpisem w historii);
// pozostałe (niewysłane) otrzymują status "expired".
func cleanupExpired(st *store.Store, logger *slog.Logger, dataDir string) {
	ctx := context.Background()

	jobs, err := st.ListExpired(ctx, time.Now())
	if err != nil {
		logger.Error("sprzątanie: pobranie listy nie powiodło się", "error", err)

		return
	}

	for _, j := range jobs {
		if j.EncryptedPath != "" {
			if rerr := os.Remove(j.EncryptedPath); rerr != nil && !os.IsNotExist(rerr) {
				logger.Warn("sprzątanie: nie udało się usunąć pliku", "job", j.UUID, "error", rerr)
			}
		}

		newStatus := "expired"
		if j.Status == "sent" {
			newStatus = "sent"
		}

		if uerr := st.MarkCleaned(ctx, j.UUID, newStatus); uerr != nil {
			logger.Error("sprzątanie: aktualizacja joba nie powiodła się", "job", j.UUID, "error", uerr)
		}
	}

	if len(jobs) > 0 {
		logger.Info("sprzątanie zakończone", "count", len(jobs), "data_dir", dataDir)
	}
}
