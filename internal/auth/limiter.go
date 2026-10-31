package auth

import (
	"sync"
	"time"
)

// Mismo esquema que el PIN del Modo LAN (internal/lan): 5 fallos por IP antes
// del primer bloqueo (30 s, se duplica hasta 15 min) y un tope global.
const (
	freeAttempts   = 5
	baseLock       = 30 * time.Second
	maxLock        = 15 * time.Minute
	globalWindow   = 10 * time.Minute
	globalMaxFails = 30
	globalLock     = 5 * time.Minute
)

type ipState struct {
	fails  int
	locked time.Time
	last   time.Time
}

type limiter struct {
	now func() time.Time
	mu  sync.Mutex
	ips map[string]*ipState

	winStart   time.Time
	winFails   int
	globalLock time.Time
}

func newLimiter(now func() time.Time) *limiter {
	return &limiter{now: now, ips: map[string]*ipState{}}
}

// allow dice si la IP puede intentarlo y, si no, cuánto debe esperar.
func (l *limiter) allow(ip string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Before(l.globalLock) {
		return false, l.globalLock.Sub(now)
	}
	if st, ok := l.ips[ip]; ok && now.Before(st.locked) {
		return false, st.locked.Sub(now)
	}
	return true, 0
}

func (l *limiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.ips) > 1000 {
		for k, st := range l.ips {
			if now.Sub(st.last) > time.Hour {
				delete(l.ips, k)
			}
		}
	}
	st := l.ips[ip]
	if st == nil {
		st = &ipState{}
		l.ips[ip] = st
	}
	st.fails++
	st.last = now
	if st.fails >= freeAttempts {
		d := baseLock << uint(min(st.fails-freeAttempts, 10))
		if d > maxLock || d <= 0 {
			d = maxLock
		}
		st.locked = now.Add(d)
	}
	if now.Sub(l.winStart) > globalWindow {
		l.winStart, l.winFails = now, 0
	}
	l.winFails++
	if l.winFails >= globalMaxFails {
		l.globalLock = now.Add(globalLock)
		l.winStart, l.winFails = now, 0
	}
}

func (l *limiter) succeed(ip string) {
	l.mu.Lock()
	delete(l.ips, ip)
	l.mu.Unlock()
}
