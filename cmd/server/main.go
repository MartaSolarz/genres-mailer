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

	"github.com/msolarzwebsensa/genres-mailer/internal/config"
	"github.com/msolarzwebsensa/genres-mailer/internal/handlers"
	"github.com/msolarzwebsensa/genres-mailer/internal/store"
)

func main() {
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
		return fmt.Errorf("otwarcie bazy: %w", err)
	}

	defer func() { _ = st.Close() }()

	logger.Info("baza otwarta, migracje zastosowane", "db_path", cfg.DBPath)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	var handler http.Handler = mux

	handler = handlers.SecurityHeaders(handler)
	handler = handlers.RequestLogger(logger, handler)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
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
