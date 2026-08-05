package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
)

type Session struct {
	Token     string
	CSRFToken string
	UserID    int64
	Username  string
	ExpiresAt time.Time
}

func (s *Session) Authenticated() bool {
	return s.UserID != 0
}

type SessionStore struct {
	mu   sync.Mutex
	ttl  time.Duration
	data map[string]*Session
}

func NewSessionStore(ttl time.Duration) *SessionStore {
	return &SessionStore{
		ttl:  ttl,
		data: make(map[string]*Session),
	}
}

func (st *SessionStore) New() (*Session, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}

	csrf, err := randomToken()
	if err != nil {
		return nil, err
	}

	sess := &Session{
		Token:     token,
		CSRFToken: csrf,
		ExpiresAt: time.Now().Add(st.ttl),
	}

	st.mu.Lock()
	st.data[token] = sess
	st.mu.Unlock()

	return sess, nil
}

func (st *SessionStore) Get(token string) (*Session, bool) {
	if token == "" {
		return nil, false
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	sess, ok := st.data[token]
	if !ok {
		return nil, false
	}

	if time.Now().After(sess.ExpiresAt) {
		delete(st.data, token)

		return nil, false
	}

	return sess, true
}

func (st *SessionStore) Delete(token string) {
	st.mu.Lock()
	delete(st.data, token)
	st.mu.Unlock()
}

func (st *SessionStore) Authenticate(oldToken string, userID int64, username string) (*Session, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}

	csrf, err := randomToken()
	if err != nil {
		return nil, err
	}

	sess := &Session{
		Token:     token,
		CSRFToken: csrf,
		UserID:    userID,
		Username:  username,
		ExpiresAt: time.Now().Add(st.ttl),
	}

	st.mu.Lock()
	delete(st.data, oldToken)
	st.data[token] = sess
	st.mu.Unlock()

	return sess, nil
}

func (st *SessionStore) GC() {
	now := time.Now()

	st.mu.Lock()
	defer st.mu.Unlock()

	for token, sess := range st.data {
		if now.After(sess.ExpiresAt) {
			delete(st.data, token)
		}
	}
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generowanie tokenu: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}
