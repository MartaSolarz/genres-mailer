package auth

import (
	"testing"
	"time"
)

func TestRateLimiterBlocksAfterMax(t *testing.T) {
	rl := NewRateLimiter(5, 15*time.Minute)
	key := "1.2.3.4|genetyk"

	for i := range 5 {
		if !rl.Allowed(key) {
			t.Fatalf("próba %d powinna być dozwolona", i+1)
		}

		rl.RecordFailure(key)
	}

	if rl.Allowed(key) {
		t.Fatal("po 5 nieudanych próbach kolejna powinna być zablokowana")
	}
}

func TestRateLimiterResetOnSuccess(t *testing.T) {
	rl := NewRateLimiter(5, 15*time.Minute)
	key := "1.2.3.4|genetyk"

	for range 5 {
		rl.RecordFailure(key)
	}

	rl.Reset(key)

	if !rl.Allowed(key) {
		t.Fatal("po resecie próby powinny być ponownie dozwolone")
	}
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	rl := NewRateLimiter(2, 10*time.Millisecond)
	key := "1.2.3.4|genetyk"

	rl.RecordFailure(key)
	rl.RecordFailure(key)

	if rl.Allowed(key) {
		t.Fatal("po 2 próbach powinno być zablokowane")
	}

	time.Sleep(20 * time.Millisecond)

	if !rl.Allowed(key) {
		t.Fatal("po upływie okna próby powinny być ponownie dozwolone")
	}
}

func TestRateLimiterKeysIndependent(t *testing.T) {
	rl := NewRateLimiter(2, 15*time.Minute)

	rl.RecordFailure("a")
	rl.RecordFailure("a")

	if !rl.Allowed("b") {
		t.Fatal("limit dla klucza a nie powinien wpływać na klucz b")
	}
}
