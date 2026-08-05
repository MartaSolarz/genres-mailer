package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()

	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Cleanup(func() { _ = st.Close() })

	return st
}

func TestCleanupExpiredJob(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	if err := st.CreateUser(ctx, "genetyk", "hash"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	user, err := st.GetUserByUsername(ctx, "genetyk")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}

	if _, err := st.UpsertSample(ctx, "PROBKA-001", "a@example.org"); err != nil {
		t.Fatalf("UpsertSample: %v", err)
	}

	job := &Job{
		UUID:              "uuid-1",
		SampleID:          "PROBKA-001",
		UserID:            user.ID,
		Status:            "encrypted",
		EncryptedPath:     "/tmp/nieistotne.pdf",
		PasswordEncrypted: []byte("zaszyfrowane-haslo"),
		CreatedAt:         time.Now().Add(-100 * time.Hour),
		ExpiresAt:         time.Now().Add(-1 * time.Hour),
	}

	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	expired, err := st.ListExpired(ctx, time.Now())
	if err != nil {
		t.Fatalf("ListExpired: %v", err)
	}

	if len(expired) != 1 || expired[0].UUID != "uuid-1" {
		t.Fatalf("ListExpired powinno zwrócić job po terminie, zwróciło %d", len(expired))
	}

	if err := st.MarkCleaned(ctx, "uuid-1", "expired"); err != nil {
		t.Fatalf("MarkCleaned: %v", err)
	}

	cleaned, err := st.GetJob(ctx, "uuid-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}

	if cleaned.Status != "expired" {
		t.Errorf("status = %q, oczekiwano expired", cleaned.Status)
	}

	if cleaned.PasswordEncrypted != nil {
		t.Error("hasło powinno zostać wyzerowane po sprzątnięciu")
	}

	if cleaned.EncryptedPath != "" {
		t.Error("ścieżka pliku powinna zostać wyzerowana po sprzątnięciu")
	}

	if again, _ := st.ListExpired(ctx, time.Now()); len(again) != 0 {
		t.Error("posprzątany job nie powinien pojawić się ponownie na liście")
	}
}
