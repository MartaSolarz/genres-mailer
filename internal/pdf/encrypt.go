package pdf

import (
	"fmt"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func init() {
	// Wyłącza katalog konfiguracyjny pdfcpu — nie tworzymy plików poza DATA_DIR
	// (istotne dla obrazów distroless i uruchomienia jako nieroot).
	api.DisableConfigDir()
}

// EncryptFile szyfruje PDF z inFile do outFile algorytmem AES-256. Hasło
// użytkownika to userPassword; hasło właściciela jest losowe i nieprzechowywane.
func EncryptFile(inFile, outFile, userPassword string) error {
	ownerPassword, err := randomString(32, alphabet)
	if err != nil {
		return err
	}

	conf := model.NewAESConfiguration(userPassword, ownerPassword, 256)

	if err := api.EncryptFile(inFile, outFile, conf); err != nil {
		return fmt.Errorf("szyfrowanie PDF: %w", err)
	}

	return nil
}
