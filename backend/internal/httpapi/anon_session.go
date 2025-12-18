package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"
)

type anonSession struct {
	jar      *cookiejar.Jar
	lastUsed time.Time
}

type anonSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*anonSession
	ttl      time.Duration
	max      int
}

func newAnonSessionStore(ttl time.Duration, max int) *anonSessionStore {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	if max <= 0 {
		max = 1000
	}
	return &anonSessionStore{
		sessions: make(map[string]*anonSession),
		ttl:      ttl,
		max:      max,
	}
}

func (s *anonSessionStore) GetOrCreate(w http.ResponseWriter, r *http.Request) (*cookiejar.Jar, error) {
	if s == nil {
		return nil, nil
	}

	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupLocked(now)

	var sid string
	if c, err := r.Cookie("se_sid"); err == nil && c != nil {
		sid = strings.TrimSpace(c.Value)
	}
	if sid != "" {
		if sess := s.sessions[sid]; sess != nil && sess.jar != nil {
			sess.lastUsed = now
			return sess.jar, nil
		}
	}

	if len(s.sessions) >= s.max {
		s.cleanupLocked(now.Add(s.ttl))
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	sid = randHex(16)
	s.sessions[sid] = &anonSession{jar: jar, lastUsed: now}

	http.SetCookie(w, &http.Cookie{
		Name:     "se_sid",
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.ttl.Seconds()),
	})

	return jar, nil
}

func (s *anonSessionStore) cleanupLocked(now time.Time) {
	if s == nil {
		return
	}
	for id, sess := range s.sessions {
		if sess == nil {
			delete(s.sessions, id)
			continue
		}
		if now.Sub(sess.lastUsed) > s.ttl {
			delete(s.sessions, id)
		}
	}
}

func randHex(bytes int) string {
	if bytes <= 0 {
		bytes = 16
	}
	buf := make([]byte, bytes)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
