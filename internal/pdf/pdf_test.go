package pdf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalPDF = "%PDF-1.4\n" +
	"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n" +
	"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n" +
	"3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]>>endobj\n" +
	"trailer<</Size 4/Root 1 0 R>>\n%%EOF\n"

func TestGeneratePasswordProperties(t *testing.T) {
	seen := make(map[string]bool)

	for range 200 {
		pw, err := GeneratePassword()
		if err != nil {
			t.Fatalf("GeneratePassword: %v", err)
		}

		if len(pw) != passwordLength {
			t.Fatalf("długość hasła = %d, oczekiwano %d", len(pw), passwordLength)
		}

		if strings.ContainsAny(pw, "0OIl1") {
			t.Fatalf("hasło zawiera znak mylący: %q", pw)
		}

		if !strings.ContainsAny(pw, letters) || !strings.ContainsAny(pw, digits) {
			t.Fatalf("hasło musi zawierać literę i cyfrę: %q", pw)
		}

		seen[pw] = true
	}

	if len(seen) < 190 {
		t.Fatalf("hasła powinny być losowe, unikalnych: %d/200", len(seen))
	}
}

func TestEncryptFile(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pdf")
	out := filepath.Join(dir, "out.pdf")

	if err := os.WriteFile(in, []byte(minimalPDF), 0o600); err != nil {
		t.Fatalf("zapis PDF: %v", err)
	}

	if err := EncryptFile(in, out, "TajneHaslo123"); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("odczyt zaszyfrowanego pliku: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("zaszyfrowany plik jest pusty")
	}

	if !strings.HasPrefix(string(data[:5]), "%PDF-") {
		t.Fatal("zaszyfrowany plik powinien pozostać prawidłowym PDF")
	}

	if !strings.Contains(string(data), "Encrypt") {
		t.Fatal("zaszyfrowany plik powinien zawierać słownik szyfrowania")
	}
}
