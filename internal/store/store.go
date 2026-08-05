package store

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/msolarzwebsensa/genres-mailer/migrations"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(dbPath string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("otwarcie bazy: %w", err)
	}

	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping bazy: %w", err)
	}

	if err := os.Chmod(dbPath, 0o600); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("ustawienie uprawnień bazy: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("migracje: %w", err)
	}

	return s, nil
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("zamknięcie bazy: %w", err)
	}

	return nil
}

func (s *Store) migrate() error {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("odczyt katalogu migracji: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}

	sort.Strings(names)

	for _, name := range names {
		content, err := migrations.FS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("odczyt migracji %s: %w", name, err)
		}

		if _, err := s.db.Exec(string(content)); err != nil {
			return fmt.Errorf("wykonanie migracji %s: %w", name, err)
		}
	}

	return nil
}
