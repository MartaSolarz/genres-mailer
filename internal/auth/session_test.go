package auth

import (
	"testing"
	"time"
)

func TestSessionNewIsAnonymous(t *testing.T) {
	st := NewSessionStore(time.Hour)

	sess, err := st.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if sess.Authenticated() {
		t.Fatal("nowa sesja nie powinna być uwierzytelniona")
	}

	if sess.Token == "" || sess.CSRFToken == "" {
		t.Fatal("sesja powinna mieć token i token CSRF")
	}
}

func TestSessionGet(t *testing.T) {
	st := NewSessionStore(time.Hour)
	sess, _ := st.New()

	got, ok := st.Get(sess.Token)
	if !ok || got.Token != sess.Token {
		t.Fatal("Get powinno zwrócić istniejącą sesję")
	}

	if _, ok := st.Get("nieistniejący"); ok {
		t.Fatal("Get dla nieznanego tokenu powinno zwrócić false")
	}
}

func TestSessionExpiry(t *testing.T) {
	st := NewSessionStore(5 * time.Millisecond)
	sess, _ := st.New()

	time.Sleep(15 * time.Millisecond)

	if _, ok := st.Get(sess.Token); ok {
		t.Fatal("wygasła sesja nie powinna być zwracana")
	}
}

func TestSessionAuthenticateRotatesToken(t *testing.T) {
	st := NewSessionStore(time.Hour)
	anon, _ := st.New()

	authed, err := st.Authenticate(anon.Token, 42, "genetyk")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}

	if authed.Token == anon.Token {
		t.Fatal("token powinien zostać zrotowany po zalogowaniu")
	}

	if !authed.Authenticated() || authed.UserID != 42 {
		t.Fatal("sesja po zalogowaniu powinna być uwierzytelniona z poprawnym UserID")
	}

	if _, ok := st.Get(anon.Token); ok {
		t.Fatal("stary token powinien zostać unieważniony")
	}
}

func TestSessionDelete(t *testing.T) {
	st := NewSessionStore(time.Hour)
	sess, _ := st.New()

	st.Delete(sess.Token)

	if _, ok := st.Get(sess.Token); ok {
		t.Fatal("usunięta sesja nie powinna być zwracana")
	}
}
