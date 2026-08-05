package pdf

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

const (
	passwordLength = 16
	letters        = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	digits         = "23456789"
	alphabet       = letters + digits
)

// GeneratePassword zwraca 16-znakowe hasło z alfabetu bez znaków mylących
// (0, O, I, l, 1), zawsze zawierające przynajmniej jedną literę i jedną cyfrę.
func GeneratePassword() (string, error) {
	for range 100 {
		pw, err := randomString(passwordLength, alphabet)
		if err != nil {
			return "", err
		}

		if strings.ContainsAny(pw, letters) && strings.ContainsAny(pw, digits) {
			return pw, nil
		}
	}

	return "", fmt.Errorf("nie udało się wygenerować hasła spełniającego wymagania")
}

func randomString(n int, charset string) (string, error) {
	b := make([]byte, n)
	max := big.NewInt(int64(len(charset)))

	for i := range b {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("losowanie znaku hasła: %w", err)
		}

		b[i] = charset[idx.Int64()]
	}

	return string(b), nil
}
