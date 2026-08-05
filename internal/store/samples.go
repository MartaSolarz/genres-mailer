package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Sample struct {
	SampleID       string
	RecipientEmail string
}

func (s *Store) UpsertSample(ctx context.Context, sampleID, email string) (inserted bool, err error) {
	var exists int

	qerr := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM samples WHERE sample_id = ?`, sampleID,
	).Scan(&exists)

	switch {
	case errors.Is(qerr, sql.ErrNoRows):
		inserted = true
	case qerr != nil:
		return false, fmt.Errorf("sprawdzenie istnienia próbki: %w", qerr)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO samples (sample_id, recipient_email, created_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(sample_id) DO UPDATE SET recipient_email = excluded.recipient_email`,
		sampleID, email, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return false, fmt.Errorf("zapis próbki: %w", err)
	}

	return inserted, nil
}

func (s *Store) GetSample(ctx context.Context, sampleID string) (*Sample, error) {
	var sm Sample

	err := s.db.QueryRowContext(ctx,
		`SELECT sample_id, recipient_email FROM samples WHERE sample_id = ?`,
		sampleID,
	).Scan(&sm.SampleID, &sm.RecipientEmail)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("pobranie próbki: %w", err)
	}

	return &sm, nil
}

func (s *Store) ListSamples(ctx context.Context) ([]Sample, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT sample_id, recipient_email FROM samples ORDER BY sample_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("lista próbek: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var out []Sample

	for rows.Next() {
		var sm Sample
		if err := rows.Scan(&sm.SampleID, &sm.RecipientEmail); err != nil {
			return nil, fmt.Errorf("odczyt próbki: %w", err)
		}

		out = append(out, sm)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iteracja próbek: %w", err)
	}

	return out, nil
}
