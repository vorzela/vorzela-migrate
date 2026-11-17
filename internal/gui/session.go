package gui

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookieName = "vm_gui_session"
	maxLoginFailures  = 5
	loginLockout      = 30 * time.Second
)

type browserSession struct {
	ID      string
	CSRF    string
	Remote  string
	Created time.Time
	Expires time.Time
}

type loginFailure struct {
	count int
	until time.Time
}

type sessionManager struct {
	mu               sync.Mutex
	sessions         map[string]*browserSession
	bootstrapToken   string
	bootstrapExpires time.Time
	ttl              time.Duration
	secureCookie     bool
	failures         map[string]*loginFailure
}

func newSessionManager(ttl time.Duration, secureCookie bool) (*sessionManager, error) {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	token, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	return &sessionManager{
		sessions:         map[string]*browserSession{},
		bootstrapToken:   token,
		bootstrapExpires: time.Now().UTC().Add(15 * time.Minute),
		ttl:              ttl,
		secureCookie:     secureCookie,
		failures:         map[string]*loginFailure{},
	}, nil
}

func randomToken(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (m *sessionManager) bootstrap() (string, time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bootstrapToken, m.bootstrapExpires
}

// exchangeBootstrap consumes the one-time token and returns a new session.
func (m *sessionManager) exchangeBootstrap(token, remote string) (*browserSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked()
	if m.bootstrapToken == "" || time.Now().UTC().After(m.bootstrapExpires) {
		return nil, errors.New("the access token has expired; restart `vm gui` to print a new one")
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(m.bootstrapToken)) != 1 {
		m.recordFailureLocked(remote)
		return nil, errors.New("invalid access token")
	}
	m.bootstrapToken = ""
	return m.newSessionLocked(remote)
}

func (m *sessionManager) newSession(remote string) (*browserSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.newSessionLocked(remote)
}

func (m *sessionManager) newSessionLocked(remote string) (*browserSession, error) {
	id, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	csrf, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	session := &browserSession{
		ID:      id,
		CSRF:    csrf,
		Remote:  remoteHost(remote),
		Created: now,
		Expires: now.Add(m.ttl),
	}
	m.sessions[id] = session
	return session, nil
}

func (m *sessionManager) get(r *http.Request) *browserSession {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked()
	session := m.sessions[cookie.Value]
	if session == nil {
		return nil
	}
	if time.Now().UTC().After(session.Expires) {
		delete(m.sessions, session.ID)
		return nil
	}
	return session
}

func (m *sessionManager) revoke(session *browserSession) {
	if session == nil {
		return
	}
	m.mu.Lock()
	delete(m.sessions, session.ID)
	m.mu.Unlock()
}

func (m *sessionManager) setCookie(w http.ResponseWriter, session *browserSession) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secureCookie,
		SameSite: http.SameSiteStrictMode,
		Expires:  session.Expires,
		MaxAge:   int(time.Until(session.Expires).Seconds()),
	})
}

func (m *sessionManager) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secureCookie,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
}

func (m *sessionManager) checkLoginAllowed(remote string) bool {
	key := remoteHost(remote)
	m.mu.Lock()
	defer m.mu.Unlock()
	failure := m.failures[key]
	if failure == nil {
		return true
	}
	if time.Now().UTC().Before(failure.until) {
		return false
	}
	if failure.count >= maxLoginFailures {
		delete(m.failures, key)
	}
	return true
}

func (m *sessionManager) recordFailure(remote string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordFailureLocked(remote)
}

func (m *sessionManager) recordFailureLocked(remote string) {
	key := remoteHost(remote)
	failure := m.failures[key]
	if failure == nil {
		failure = &loginFailure{}
		m.failures[key] = failure
	}
	failure.count++
	if failure.count >= maxLoginFailures {
		failure.until = time.Now().UTC().Add(loginLockout)
	}
}

func (m *sessionManager) cleanupLocked() {
	now := time.Now().UTC()
	for id, session := range m.sessions {
		if now.After(session.Expires) {
			delete(m.sessions, id)
		}
	}
	for key, failure := range m.failures {
		if !failure.until.IsZero() && now.After(failure.until) {
			delete(m.failures, key)
		}
	}
}

func remoteHost(remote string) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return remote
	}
	return host
}
