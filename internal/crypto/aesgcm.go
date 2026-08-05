// Package crypto zapewnia symetryczne szyfrowanie AES-GCM używane do
// przechowywania haseł PDF w bazie (kluczem APP_SECRET_KEY).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

// Seal szyfruje plaintext kluczem key (32 bajty). Wynik zawiera nonce
// na początku i jest gotowy do zapisu w bazie.
func Seal(key, plaintext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("losowanie nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Open odszyfrowuje dane zaszyfrowane przez Seal.
func Open(key, ciphertext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}

	ns := gcm.NonceSize()
	if len(ciphertext) < ns {
		return nil, fmt.Errorf("szyfrogram za krótki")
	}

	nonce, ct := ciphertext[:ns], ciphertext[ns:]

	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("odszyfrowanie: %w", err)
	}

	return plaintext, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("utworzenie szyfru AES: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("utworzenie GCM: %w", err)
	}

	return gcm, nil
}
