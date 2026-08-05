package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Job struct {
	ID                int64
	UUID              string
	SampleID          string
	UserID            int64
	Status            string
	EncryptedPath     string
	PasswordEncrypted []byte
	CreatedAt         time.Time
	SentAt            *time.Time
	ExpiresAt         time.Time
}

func (s *Store) CreateJob(ctx context.Context, j *Job) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO jobs (uuid, sample_id, user_id, status, encrypted_path, password_encrypted, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		j.UUID, j.SampleID, j.UserID, j.Status, j.EncryptedPath, j.PasswordEncrypted,
		j.CreatedAt.UTC().Format(time.RFC3339), j.ExpiresAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("utworzenie joba: %w", err)
	}

	return nil
}

// GetJob zwraca job po UUID. Dostęp jest współdzielony — dokument jest widoczny
// dla wszystkich zalogowanych użytkowników; każdy dostęp powinien być odnotowany
// w audit_log przez warstwę handlerów. Dla nieznanego UUID zwraca ErrNotFound.
func (s *Store) GetJob(ctx context.Context, uuid string) (*Job, error) {
	var (
		j          Job
		encPath    sql.NullString
		passwd     []byte
		createdRaw string
		sentRaw    sql.NullString
		expiresRaw string
	)

	err := s.db.QueryRowContext(ctx,
		`SELECT id, uuid, sample_id, user_id, status, encrypted_path, password_encrypted,
		        created_at, sent_at, expires_at
		 FROM jobs WHERE uuid = ?`,
		uuid,
	).Scan(&j.ID, &j.UUID, &j.SampleID, &j.UserID, &j.Status, &encPath, &passwd,
		&createdRaw, &sentRaw, &expiresRaw)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("pobranie joba: %w", err)
	}

	j.EncryptedPath = encPath.String
	j.PasswordEncrypted = passwd
	j.CreatedAt = parseTime(createdRaw)
	j.ExpiresAt = parseTime(expiresRaw)

	if sentRaw.Valid {
		t := parseTime(sentRaw.String)
		j.SentAt = &t
	}

	return &j, nil
}

func (s *Store) MarkJobSent(ctx context.Context, uuid string, sentAt time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status = 'sent', sent_at = ? WHERE uuid = ?`,
		sentAt.UTC().Format(time.RFC3339), uuid,
	)
	if err != nil {
		return fmt.Errorf("oznaczenie joba jako wysłany: %w", err)
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

func (s *Store) UpdateJobStatus(ctx context.Context, uuid, status string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE jobs SET status = ? WHERE uuid = ?`, status, uuid)
	if err != nil {
		return fmt.Errorf("zmiana statusu joba: %w", err)
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

type ExpiredJob struct {
	UUID          string
	EncryptedPath string
	Status        string
}

func (s *Store) ListExpired(ctx context.Context, now time.Time) ([]ExpiredJob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT uuid, COALESCE(encrypted_path, ''), status
		 FROM jobs
		 WHERE expires_at < ? AND (encrypted_path IS NOT NULL OR password_encrypted IS NOT NULL)`,
		now.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("lista jobów do sprzątania: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var out []ExpiredJob

	for rows.Next() {
		var e ExpiredJob
		if err := rows.Scan(&e.UUID, &e.EncryptedPath, &e.Status); err != nil {
			return nil, fmt.Errorf("odczyt joba do sprzątania: %w", err)
		}

		out = append(out, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iteracja jobów do sprzątania: %w", err)
	}

	return out, nil
}

func (s *Store) MarkCleaned(ctx context.Context, uuid, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET encrypted_path = NULL, password_encrypted = NULL, status = ? WHERE uuid = ?`,
		status, uuid,
	)
	if err != nil {
		return fmt.Errorf("oznaczenie joba jako posprzątany: %w", err)
	}

	return nil
}

func (s *Store) DeleteJob(ctx context.Context, uuid string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE uuid = ?`, uuid)
	if err != nil {
		return fmt.Errorf("usunięcie joba: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("liczba usuniętych wierszy: %w", err)
	}

	if n == 0 {
		return ErrNotFound
	}

	return nil
}

type JobSummary struct {
	UUID      string
	SampleID  string
	Username  string
	Status    string
	CreatedAt time.Time
	SentAt    *time.Time
}

// ListJobs zwraca ostatnie joby wszystkich użytkowników (dostęp współdzielony)
// wraz z loginem osoby, która utworzyła dokument.
func (s *Store) ListJobs(ctx context.Context, limit int) ([]JobSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT j.uuid, j.sample_id, u.username, j.status, j.created_at, j.sent_at
		 FROM jobs j JOIN users u ON u.id = j.user_id
		 ORDER BY j.created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("lista jobów: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var out []JobSummary

	for rows.Next() {
		var (
			js         JobSummary
			createdRaw string
			sentRaw    sql.NullString
		)

		if err := rows.Scan(&js.UUID, &js.SampleID, &js.Username, &js.Status, &createdRaw, &sentRaw); err != nil {
			return nil, fmt.Errorf("odczyt joba: %w", err)
		}

		js.CreatedAt = parseTime(createdRaw)

		if sentRaw.Valid {
			t := parseTime(sentRaw.String)
			js.SentAt = &t
		}

		out = append(out, js)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iteracja jobów: %w", err)
	}

	return out, nil
}

func (s *Store) CountJobsByUserSince(ctx context.Context, userID int64, since time.Time) (int, error) {
	var n int

	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM jobs WHERE user_id = ? AND created_at >= ?`,
		userID, since.UTC().Format(time.RFC3339),
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("zliczanie jobów: %w", err)
	}

	return n, nil
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}

	return t
}
