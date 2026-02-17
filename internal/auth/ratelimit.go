package auth

import (
	"net"
	"sync"
	"time"
)

// RateLimiter tracks login attempts per IP
type RateLimiter struct {
	mu       sync.RWMutex
	attempts map[string]*attemptInfo
}

type attemptInfo struct {
	count     int
	lastReset time.Time
}

const (
	maxAttempts   = 5
	windowSeconds = 60
)

// NewRateLimiter creates a new rate limiter
func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		attempts: make(map[string]*attemptInfo),
	}
	// Start cleanup goroutine
	go rl.cleanup()
	return rl
}

// Allow checks if a login attempt from the given IP should be allowed
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	info, exists := rl.attempts[ip]

	if !exists || now.Sub(info.lastReset) > windowSeconds*time.Second {
		// First attempt or window expired
		rl.attempts[ip] = &attemptInfo{
			count:     1,
			lastReset: now,
		}
		return true
	}

	if info.count >= maxAttempts {
		return false
	}

	info.count++
	return true
}

// GetRemainingAttempts returns the number of remaining attempts for an IP
func (rl *RateLimiter) GetRemainingAttempts(ip string) int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	info, exists := rl.attempts[ip]
	if !exists {
		return maxAttempts
	}

	if time.Since(info.lastReset) > windowSeconds*time.Second {
		return maxAttempts
	}

	remaining := maxAttempts - info.count
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Reset clears the rate limit for an IP
func (rl *RateLimiter) Reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, ip)
}

// cleanup periodically removes expired entries
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, info := range rl.attempts {
			if now.Sub(info.lastReset) > windowSeconds*time.Second {
				delete(rl.attempts, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// GetClientIP extracts the client IP from the request
func GetClientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
