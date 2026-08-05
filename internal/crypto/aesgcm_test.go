package crypto

import (
	"bytes"
	"testing"
)

func key32() []byte {
	return bytes.Repeat([]byte("k"), 32)
}

func TestSealOpenRoundtrip(t *testing.T) {
	key := key32()
	plaintext := []byte("HasloDoPDF-2X9k")

	sealed, err := Seal(key, plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	if bytes.Contains(sealed, plaintext) {
		t.Fatal("szyfrogram nie powinien zawierać jawnego tekstu")
	}

	opened, err := Open(key, sealed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if !bytes.Equal(opened, plaintext) {
		t.Fatalf("odszyfrowano %q, oczekiwano %q", opened, plaintext)
	}
}

func TestOpenTamperedFails(t *testing.T) {
	key := key32()

	sealed, err := Seal(key, []byte("dane"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	sealed[len(sealed)-1] ^= 0xff

	if _, err := Open(key, sealed); err == nil {
		t.Fatal("naruszony szyfrogram powinien zostać odrzucony")
	}
}

func TestOpenWrongKeyFails(t *testing.T) {
	sealed, err := Seal(key32(), []byte("dane"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	wrong := bytes.Repeat([]byte("x"), 32)
	if _, err := Open(wrong, sealed); err == nil {
		t.Fatal("odszyfrowanie złym kluczem powinno się nie udać")
	}
}
