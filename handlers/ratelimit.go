package handlers

import (
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// RateLimitConfig holds configuration from environment variables
type RateLimitConfig struct {
	MaxAttempts     int
	WindowDuration  time.Duration
	LockoutDuration time.Duration
}

// LoginAttempt tracks login attempts for a single IP
type LoginAttempt struct {
	Count        int
	FirstAttempt time.Time
	LockedUntil  time.Time
}

// LoginRateLimiter manages rate limiting for login attempts
type LoginRateLimiter struct {
	config   RateLimitConfig
	attempts map[string]*LoginAttempt
	mu       sync.RWMutex
}

// Singleton instance
var loginLimiter *LoginRateLimiter

// InitLoginRateLimiter initializes the rate limiter with env vars
func InitLoginRateLimiter() {
	config := RateLimitConfig{
		MaxAttempts:     getEnvInt("LOGIN_MAX_ATTEMPTS", 5),
		WindowDuration:  time.Duration(getEnvInt("LOGIN_WINDOW_MINUTES", 15)) * time.Minute,
		LockoutDuration: time.Duration(getEnvInt("LOGIN_LOCKOUT_MINUTES", 30)) * time.Minute,
	}

	loginLimiter = &LoginRateLimiter{
		config:   config,
		attempts: make(map[string]*LoginAttempt),
	}

	// Start cleanup goroutine
	go loginLimiter.cleanupRoutine()

	log.Printf("[RATE LIMIT] Initialized: max=%d attempts per %v, lockout=%v",
		config.MaxAttempts, config.WindowDuration, config.LockoutDuration)
}

func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	intVal, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return intVal
}

// IsBlocked checks if an IP is currently blocked
func (rl *LoginRateLimiter) IsBlocked(ip string) (bool, time.Duration) {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	attempt, exists := rl.attempts[ip]
	if !exists {
		return false, 0
	}

	if time.Now().Before(attempt.LockedUntil) {
		remaining := time.Until(attempt.LockedUntil)
		return true, remaining
	}

	return false, 0
}

// RecordAttempt records a failed login attempt, returns true if limit exceeded
func (rl *LoginRateLimiter) RecordAttempt(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	attempt, exists := rl.attempts[ip]

	if !exists {
		rl.attempts[ip] = &LoginAttempt{
			Count:        1,
			FirstAttempt: now,
		}
		return false
	}

	// Check if window has expired
	if now.Sub(attempt.FirstAttempt) > rl.config.WindowDuration {
		attempt.Count = 1
		attempt.FirstAttempt = now
		attempt.LockedUntil = time.Time{}
		return false
	}

	attempt.Count++

	// Check if limit exceeded
	if attempt.Count > rl.config.MaxAttempts {
		attempt.LockedUntil = now.Add(rl.config.LockoutDuration)
		log.Printf("[RATE LIMIT] IP %s blocked until %s (attempts: %d)",
			ip, attempt.LockedUntil.Format("15:04:05"), attempt.Count)
		return true
	}

	return false
}

// ResetAttempts clears the attempt counter after successful login
func (rl *LoginRateLimiter) ResetAttempts(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, ip)
}

// cleanupRoutine periodically removes old entries
func (rl *LoginRateLimiter) cleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.cleanup()
	}
}

func (rl *LoginRateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	maxAge := rl.config.WindowDuration + rl.config.LockoutDuration

	for ip, attempt := range rl.attempts {
		if now.Sub(attempt.FirstAttempt) > maxAge && now.After(attempt.LockedUntil) {
			delete(rl.attempts, ip)
		}
	}
}

// LoginRateLimitMiddleware checks if IP is blocked before allowing login
func LoginRateLimitMiddleware(c *fiber.Ctx) error {
	if loginLimiter == nil {
		return c.Next()
	}

	ip := c.IP()
	username := strings.ToLower(strings.TrimSpace(c.FormValue("username")))
	keys := []string{"ip:" + ip, "user:" + username}
	for _, key := range keys {
		if blocked, remaining := loginLimiter.IsBlocked(key); blocked {
			minutes := int(remaining.Minutes())
			if minutes < 1 {
				minutes = 1
			}
			log.Printf("[RATE LIMIT] Blocked login attempt from IP: %s (remaining: %dm)", ip, minutes)
			return c.Redirect("/login?error=rate_limited")
		}
	}

	return c.Next()
}
