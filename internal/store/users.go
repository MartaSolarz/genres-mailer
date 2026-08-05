package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Disabled     bool
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, created_at, disabled) VALUES (?, ?, ?, 0)`,
		username, passwordHash, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("utworzenie użytkownika: %w", err)
	}

	return nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var (
		u        User
		disabled int
	)

	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, disabled FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &disabled)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("pobranie użytkownika: %w", err)
	}

	u.Disabled = disabled != 0

	return &u, nil
}

func (s *Store) UpdatePassword(ctx context.Context, username, passwordHash string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ? WHERE username = ?`,
		passwordHash, username,
	)
	if err != nil {
		return fmt.Errorf("zmiana hasła: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("liczba zmienionych wierszy: %w", err)
	}

	if n == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *Store) SetUserDisabled(ctx context.Context, username string, disabled bool) error {
	val := 0
	if disabled {
		val = 1
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET disabled = ? WHERE username = ?`,
		val, username,
	)
	if err != nil {
		return fmt.Errorf("zmiana statusu użytkownika: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("liczba zmienionych wierszy: %w", err)
	}

	if n == 0 {
		return ErrNotFound
	}

	return nil
}
