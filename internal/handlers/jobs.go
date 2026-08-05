package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	appcrypto "github.com/msolarzwebsensa/genres-mailer/internal/crypto"
	"github.com/msolarzwebsensa/genres-mailer/internal/pdf"
	"github.com/msolarzwebsensa/genres-mailer/internal/store"
)

var errNotPDF = errors.New("plik nie jest dokumentem PDF")

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	reqID := RequestID(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes()+(1<<20))

	if err := r.ParseMultipartForm(8 << 20); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Nieprawidłowe dane lub przekroczony rozmiar pliku.")

		return
	}

	sampleID := strings.TrimSpace(r.FormValue("sample_id"))

	sample, err := s.store.GetSample(r.Context(), sampleID)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Nieznane ID próbki.")

		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Brak pliku PDF.")

		return
	}

	defer func() { _ = file.Close() }()

	if !strings.EqualFold(filepath.Ext(header.Filename), ".pdf") {
		s.writeJSONError(w, http.StatusBadRequest, "Dozwolone są wyłącznie pliki .pdf.")

		return
	}

	dayStart := time.Now().Truncate(24 * time.Hour)

	count, err := s.store.CountJobsByUserSince(r.Context(), sess.UserID, dayStart)
	if err != nil {
		s.serverError(w, reqID, "zliczanie jobów", err)

		return
	}

	if count >= s.cfg.MaxJobsPerDay {
		s.writeJSONError(w, http.StatusTooManyRequests, "Przekroczono dzienny limit dokumentów.")

		return
	}

	jobUUID := newUUID()
	tmpPath := filepath.Join(s.cfg.DataDir, "tmp", jobUUID+".pdf")
	encPath := filepath.Join(s.cfg.DataDir, "encrypted", jobUUID+".pdf")

	if err := saveUploadWithMagic(file, tmpPath); err != nil {
		_ = os.Remove(tmpPath)

		if errors.Is(err, errNotPDF) {
			s.writeJSONError(w, http.StatusBadRequest, "Plik nie jest prawidłowym dokumentem PDF.")

			return
		}

		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			s.writeJSONError(w, http.StatusRequestEntityTooLarge, "Plik przekracza dozwolony rozmiar.")

			return
		}

		s.serverError(w, reqID, "zapis pliku tymczasowego", err)

		return
	}

	// Gwarancja usunięcia niezaszyfrowanego oryginału niezależnie od dalszych błędów.
	defer func() { _ = os.Remove(tmpPath) }()

	password, err := pdf.GeneratePassword()
	if err != nil {
		s.serverError(w, reqID, "generowanie hasła", err)

		return
	}

	if err := pdf.EncryptFile(tmpPath, encPath, password); err != nil {
		_ = os.Remove(encPath)

		s.serverError(w, reqID, "szyfrowanie PDF", err)

		return
	}

	if err := os.Remove(tmpPath); err != nil {
		s.logger.Warn("nie udało się usunąć pliku tymczasowego", "request_id", reqID, "error", err)
	}

	encPassword, err := appcrypto.Seal(s.cfg.AppSecretKey, []byte(password))
	if err != nil {
		_ = os.Remove(encPath)

		s.serverError(w, reqID, "szyfrowanie hasła", err)

		return
	}

	now := time.Now()
	job := &store.Job{
		UUID:              jobUUID,
		SampleID:          sampleID,
		UserID:            sess.UserID,
		Status:            "encrypted",
		EncryptedPath:     encPath,
		PasswordEncrypted: encPassword,
		CreatedAt:         now,
		ExpiresAt:         now.Add(s.cfg.FileRetention),
	}

	if err := s.store.CreateJob(r.Context(), job); err != nil {
		_ = os.Remove(encPath)

		s.serverError(w, reqID, "zapis joba", err)

		return
	}

	if err := s.store.InsertAudit(r.Context(), &sess.UserID, "job_created", sampleID, jobUUID); err != nil {
		s.logger.Error("zapis audytu nie powiódł się", "request_id", reqID, "error", err)
	}

	s.logger.Info("utworzono zaszyfrowany dokument",
		"request_id", reqID, "user", sess.Username, "sample_id", sampleID, "job", jobUUID)

	s.writeJSON(w, http.StatusOK, map[string]any{
		"job_uuid":         jobUUID,
		"sample_id":        sampleID,
		"recipient_masked": maskEmail(sample.RecipientEmail),
	})
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	job, ok := s.jobForRequest(w, r)
	if !ok {
		return
	}

	s.auditAccess(r, "job_preview", job)
	s.serveEncrypted(w, r, job, "inline", "dokument.pdf")
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	job, ok := s.jobForRequest(w, r)
	if !ok {
		return
	}

	s.auditAccess(r, "job_download", job)
	s.serveEncrypted(w, r, job, "attachment", "wyniki-"+safeFilename(job.SampleID)+".pdf")
}

func (s *Server) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	reqID := RequestID(r.Context())
	uuid := r.PathValue("uuid")

	job, err := s.store.GetJob(r.Context(), uuid)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "Nie znaleziono", http.StatusNotFound)

			return
		}

		s.serverError(w, reqID, "pobranie joba", err)

		return
	}

	if job.Status == "sent" {
		s.writeJSONError(w, http.StatusConflict, "Nie można usunąć wysłanego dokumentu.")

		return
	}

	if job.EncryptedPath != "" {
		if err := os.Remove(job.EncryptedPath); err != nil && !os.IsNotExist(err) {
			s.logger.Warn("nie udało się usunąć pliku dokumentu", "request_id", reqID, "error", err)
		}
	}

	if err := s.store.DeleteJob(r.Context(), uuid); err != nil {
		s.serverError(w, reqID, "usunięcie joba", err)

		return
	}

	if err := s.store.InsertAudit(r.Context(), &sess.UserID, "job_deleted", job.SampleID, uuid); err != nil {
		s.logger.Error("zapis audytu nie powiódł się", "request_id", reqID, "error", err)
	}

	s.logger.Info("usunięto dokument",
		"request_id", reqID, "user", sess.Username, "sample_id", job.SampleID, "job", uuid)
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	reqID := RequestID(r.Context())
	uuid := r.PathValue("uuid")

	job, err := s.store.GetJob(r.Context(), uuid)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "Nie znaleziono", http.StatusNotFound)

			return
		}

		s.serverError(w, reqID, "pobranie joba", err)

		return
	}

	if job.Status == "sent" {
		s.writeJSONError(w, http.StatusConflict, "Dokument został już wysłany.")

		return
	}

	if job.Status != "encrypted" && job.Status != "partial" {
		s.writeJSONError(w, http.StatusConflict, "Dokument nie jest gotowy do wysłania.")

		return
	}

	sample, err := s.store.GetSample(r.Context(), job.SampleID)
	if err != nil {
		s.serverError(w, reqID, "pobranie próbki", err)

		return
	}

	password, err := appcrypto.Open(s.cfg.AppSecretKey, job.PasswordEncrypted)
	if err != nil {
		s.serverError(w, reqID, "odszyfrowanie hasła", err)

		return
	}

	// Mail 1 (dokument) wysyłamy tylko gdy nie został jeszcze wysłany — status
	// "partial" oznacza, że dokument już poszedł i ponawiamy wyłącznie hasło.
	if job.Status == "encrypted" {
		data, rerr := os.ReadFile(job.EncryptedPath)
		if rerr != nil {
			s.serverError(w, reqID, "odczyt zaszyfrowanego pliku", rerr)

			return
		}

		if merr := s.mailer.SendDocument(sample.RecipientEmail, data, "dokument.pdf"); merr != nil {
			s.logger.Error("wysyłka dokumentu nie powiodła się",
				"request_id", reqID, "job", uuid, "error", merr)
			s.writeJSON(w, http.StatusBadGateway, map[string]any{
				"error": "Nie udało się wysłać dokumentu. Spróbuj ponownie.",
				"stage": "document",
			})

			return
		}

		if uerr := s.store.UpdateJobStatus(r.Context(), uuid, "partial"); uerr != nil {
			s.serverError(w, reqID, "zapis statusu partial", uerr)

			return
		}

		s.auditAccess(r, "mail_document_sent", job)
	}

	if merr := s.mailer.SendPassword(sample.RecipientEmail, string(password)); merr != nil {
		s.logger.Error("wysyłka hasła nie powiodła się",
			"request_id", reqID, "job", uuid, "error", merr)
		s.writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": "Dokument wysłano, ale nie udało się wysłać hasła. Ponów wysłanie hasła.",
			"stage": "password",
		})

		return
	}

	if err := s.store.MarkJobSent(r.Context(), uuid, time.Now()); err != nil {
		s.serverError(w, reqID, "oznaczenie joba jako wysłany", err)

		return
	}

	s.auditAccess(r, "job_sent", job)
	s.logger.Info("dokument wysłany",
		"request_id", reqID, "user", sess.Username, "sample_id", job.SampleID, "job", uuid)
	s.writeJSON(w, http.StatusOK, map[string]any{"status": "sent"})
}

func (s *Server) auditAccess(r *http.Request, action string, job *store.Job) {
	sess := SessionFromContext(r.Context())
	if err := s.store.InsertAudit(r.Context(), &sess.UserID, action, job.SampleID, job.UUID); err != nil {
		s.logger.Error("zapis audytu nie powiódł się", "request_id", RequestID(r.Context()), "error", err)
	}
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	job, ok := s.jobForRequest(w, r)
	if !ok {
		return
	}

	reqID := RequestID(r.Context())
	sess := SessionFromContext(r.Context())

	if len(job.PasswordEncrypted) == 0 {
		s.writeJSONError(w, http.StatusGone, "Hasło nie jest już dostępne.")

		return
	}

	password, err := appcrypto.Open(s.cfg.AppSecretKey, job.PasswordEncrypted)
	if err != nil {
		s.serverError(w, reqID, "odszyfrowanie hasła", err)

		return
	}

	if err := s.store.InsertAudit(r.Context(), &sess.UserID, "password_viewed", job.SampleID, job.UUID); err != nil {
		s.logger.Error("zapis audytu nie powiódł się", "request_id", reqID, "error", err)
	}

	s.writeJSON(w, http.StatusOK, map[string]any{"password": string(password)})
}

func (s *Server) jobForRequest(w http.ResponseWriter, r *http.Request) (*store.Job, bool) {
	uuid := r.PathValue("uuid")

	job, err := s.store.GetJob(r.Context(), uuid)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "Nie znaleziono", http.StatusNotFound)

			return nil, false
		}

		s.serverError(w, RequestID(r.Context()), "pobranie joba", err)

		return nil, false
	}

	// Treść dokumentu (podgląd/pobranie/hasło) jest dostępna do czasu wysyłki.
	// Po wysłaniu lub wygaśnięciu dokument jest już tylko wpisem w historii.
	if job.Status != "encrypted" && job.Status != "partial" {
		http.Error(w, "Dokument nie jest już dostępny do podglądu.", http.StatusForbidden)

		return nil, false
	}

	return job, true
}

func (s *Server) serveEncrypted(w http.ResponseWriter, r *http.Request, job *store.Job, disposition, filename string) {
	f, err := os.Open(job.EncryptedPath)
	if err != nil {
		s.serverError(w, RequestID(r.Context()), "otwarcie pliku", err)

		return
	}

	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		s.serverError(w, RequestID(r.Context()), "stat pliku", err)

		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, filename))
	http.ServeContent(w, r, filename, info.ModTime(), f)
}

func saveUploadWithMagic(src io.Reader, dst string) error {
	head := make([]byte, 5)

	if _, err := io.ReadFull(src, head); err != nil {
		return errNotPDF
	}

	if string(head) != "%PDF-" {
		return errNotPDF
	}

	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("utworzenie pliku tymczasowego: %w", err)
	}

	defer func() { _ = f.Close() }()

	if _, err := f.Write(head); err != nil {
		return fmt.Errorf("zapis nagłówka: %w", err)
	}

	if _, err := io.Copy(f, src); err != nil {
		return fmt.Errorf("zapis pliku: %w", err)
	}

	return nil
}

func maskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "***"
	}

	local, domain := email[:at], email[at+1:]
	if len(local) <= 1 {
		return "*@" + domain
	}

	return local[:1] + "***@" + domain
}

func safeFilename(s string) string {
	var b strings.Builder

	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}

	if b.Len() == 0 {
		return "dokument"
	}

	return b.String()
}

func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString(b)
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.logger.Error("kodowanie JSON nie powiodło się", "error", err)
	}
}

func (s *Server) writeJSONError(w http.ResponseWriter, status int, message string) {
	s.writeJSON(w, status, map[string]any{"error": message})
}

func (s *Server) serverError(w http.ResponseWriter, reqID, context string, err error) {
	s.logger.Error("błąd serwera", "request_id", reqID, "context", context, "error", err)
	s.writeJSONError(w, http.StatusInternalServerError, "Wystąpił błąd serwera.")
}
