package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (s *Store) InsertAudit(ctx context.Context, userID *int64, action, sampleID, detail string) error {
	var uid sql.NullInt64
	if userID != nil {
		uid = sql.NullInt64{Int64: *userID, Valid: true}
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log (timestamp, user_id, action, sample_id, detail) VALUES (?, ?, ?, ?, ?)`,
		time.Now().UTC().Format(time.RFC3339), uid, action, nullIfEmpty(sampleID), nullIfEmpty(detail),
	)
	if err != nil {
		return fmt.Errorf("zapis audytu: %w", err)
	}

	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}

	return s
}
