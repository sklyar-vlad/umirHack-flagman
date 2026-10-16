package auth

import (
	"sync"
	"time"

	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
)

const (
	loginWindow  = 15 * time.Minute
	maxLoginKeys = 4096
)

type loginAttempts struct {
	count   int
	expires time.Time
}
type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]loginAttempts
	now     func() time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{entries: make(map[string]loginAttempts), now: time.Now}
}

func (l *loginLimiter) allow(kind, value string, limit int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for key, entry := range l.entries {
		if !now.Before(entry.expires) {
			delete(l.entries, key)
		}
	}
	key := kind + ":" + authservice.SessionTokenHash(value)
	entry, exists := l.entries[key]
	if !exists {
		if len(l.entries) >= maxLoginKeys {
			return false
		}
		entry.expires = now.Add(loginWindow)
	}
	if entry.count >= limit {
		return false
	}
	entry.count++
	l.entries[key] = entry
	return true
}
