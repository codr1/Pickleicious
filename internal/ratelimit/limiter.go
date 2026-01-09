// Package ratelimit provides rate limiting for OTP operations.
package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Clock interface for testing time-dependent behavior.
type Clock interface {
	Now() time.Time
}

// realClock implements Clock using the system time.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Config holds rate limit configuration.
type Config struct {
	// OTP Send limits
	SendCooldown     time.Duration // Minimum time between OTP sends (default: 60s)
	SendMaxPerHour   int           // Max OTP sends per identifier per hour (default: 5)
	SendMaxIPPerHour int           // Max OTP sends per IP per hour (default: 20)

	// OTP Verify limits
	VerifyMaxAttempts  int           // Max verify attempts before lockout (default: 5)
	VerifyLockout      time.Duration // Lockout duration after max attempts (default: 5m)
	VerifyMaxIPPerHour int           // Max verify attempts per IP per hour (default: 30)

	// Clock for testing (nil uses real time)
	Clock Clock
}

// DefaultConfig returns production-ready defaults.
func DefaultConfig() *Config {
	return &Config{
		SendCooldown:       60 * time.Second,
		SendMaxPerHour:     5,
		SendMaxIPPerHour:   20,
		VerifyMaxAttempts:  5,
		VerifyLockout:      5 * time.Minute,
		VerifyMaxIPPerHour: 30,
	}
}

// LimitResult contains the result of a rate limit check.
type LimitResult struct {
	Allowed    bool
	RetryAfter time.Duration
	Reason     string // For logging
}

// entry tracks request counts and timestamps.
type entry struct {
	count    int
	firstAt  time.Time // First request in window
	lastAt   time.Time // Most recent request (for cooldown)
	lockedAt time.Time // When lockout started (zero if not locked)
}

// Limiter implements multi-layer rate limiting for OTP operations.
type Limiter struct {
	config *Config
	clock  Clock
	mu     sync.RWMutex
	// Keyed by hash of identifier or IP
	sendByID   map[string]*entry
	sendByIP   map[string]*entry
	verifyByID map[string]*entry
	verifyByIP map[string]*entry

	// Cleanup goroutine management
	cleanupCtx    context.Context
	cleanupCancel context.CancelFunc
	cleanupOnce   sync.Once
	cleanupWg     sync.WaitGroup
}

// New creates a new rate limiter with the given config.
func New(cfg *Config) *Limiter {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	clock := cfg.Clock
	if clock == nil {
		clock = realClock{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Limiter{
		config:        cfg,
		clock:         clock,
		sendByID:      make(map[string]*entry),
		sendByIP:      make(map[string]*entry),
		verifyByID:    make(map[string]*entry),
		verifyByIP:    make(map[string]*entry),
		cleanupCtx:    ctx,
		cleanupCancel: cancel,
	}
}

// Close stops the cleanup goroutine and releases resources.
func (l *Limiter) Close() {
	l.cleanupCancel()
	l.cleanupWg.Wait()
}

// CheckOTPSend checks if an OTP send request is allowed.
// Does NOT record the attempt - call RecordOTPSend after successful user validation.
func (l *Limiter) CheckOTPSend(identifier, ip string) LimitResult {
	l.startCleanup()
	now := l.clock.Now()
	idKey := l.hashKey("send:id:", normalizeIdentifier(identifier))
	ipKey := l.hashKey("send:ip:", ip)

	l.mu.RLock()
	defer l.mu.RUnlock()

	// Check per-identifier cooldown
	if e := l.sendByID[idKey]; e != nil {
		elapsed := now.Sub(e.lastAt)
		if elapsed < l.config.SendCooldown {
			remaining := l.config.SendCooldown - elapsed
			return LimitResult{
				Allowed:    false,
				RetryAfter: remaining,
				Reason:     "cooldown",
			}
		}

		// Check hourly limit
		if now.Sub(e.firstAt) < time.Hour && e.count >= l.config.SendMaxPerHour {
			return LimitResult{
				Allowed:    false,
				RetryAfter: time.Hour - now.Sub(e.firstAt),
				Reason:     "hourly_limit",
			}
		}
	}

	// Check per-IP hourly limit
	if e := l.sendByIP[ipKey]; e != nil {
		if now.Sub(e.firstAt) < time.Hour && e.count >= l.config.SendMaxIPPerHour {
			return LimitResult{
				Allowed:    false,
				RetryAfter: time.Hour - now.Sub(e.firstAt),
				Reason:     "ip_hourly_limit",
			}
		}
	}

	return LimitResult{Allowed: true}
}

// RecordOTPSend records a successful OTP send. Call this AFTER user validation succeeds.
func (l *Limiter) RecordOTPSend(identifier, ip string) {
	now := l.clock.Now()
	idKey := l.hashKey("send:id:", normalizeIdentifier(identifier))
	ipKey := l.hashKey("send:ip:", ip)

	l.mu.Lock()
	defer l.mu.Unlock()

	l.recordSend(idKey, ipKey, now)
}

// CheckOTPVerify checks if an OTP verify attempt is allowed.
// Does NOT record the attempt - call RecordOTPVerify after checking the code.
func (l *Limiter) CheckOTPVerify(identifier, ip string) LimitResult {
	l.startCleanup()
	now := l.clock.Now()
	idKey := l.hashKey("verify:id:", normalizeIdentifier(identifier))
	ipKey := l.hashKey("verify:ip:", ip)

	l.mu.RLock()
	defer l.mu.RUnlock()

	// Check if identifier is locked out
	if e := l.verifyByID[idKey]; e != nil {
		if !e.lockedAt.IsZero() {
			elapsed := now.Sub(e.lockedAt)
			if elapsed < l.config.VerifyLockout {
				return LimitResult{
					Allowed:    false,
					RetryAfter: l.config.VerifyLockout - elapsed,
					Reason:     "lockout",
				}
			}
			// Lockout expired - will be cleaned up, allow this request
		} else if e.count >= l.config.VerifyMaxAttempts {
			// Already at max attempts, lockout should be started
			return LimitResult{
				Allowed:    false,
				RetryAfter: l.config.VerifyLockout,
				Reason:     "max_attempts",
			}
		}
	}

	// Check per-IP hourly limit
	if e := l.verifyByIP[ipKey]; e != nil {
		if now.Sub(e.firstAt) < time.Hour && e.count >= l.config.VerifyMaxIPPerHour {
			return LimitResult{
				Allowed:    false,
				RetryAfter: time.Hour - now.Sub(e.firstAt),
				Reason:     "ip_hourly_limit",
			}
		}
	}

	return LimitResult{Allowed: true}
}

// RecordOTPVerify records an OTP verify attempt. Call this AFTER validating the user exists.
// Returns true if max attempts reached and lockout was triggered.
func (l *Limiter) RecordOTPVerify(identifier, ip string) (lockedOut bool) {
	now := l.clock.Now()
	idKey := l.hashKey("verify:id:", normalizeIdentifier(identifier))
	ipKey := l.hashKey("verify:ip:", ip)

	l.mu.Lock()
	defer l.mu.Unlock()

	// Update identifier entry
	e := l.verifyByID[idKey]
	if e == nil {
		l.verifyByID[idKey] = &entry{count: 1, firstAt: now, lastAt: now}
	} else if !e.lockedAt.IsZero() && now.Sub(e.lockedAt) >= l.config.VerifyLockout {
		// Lockout expired, reset
		l.verifyByID[idKey] = &entry{count: 1, firstAt: now, lastAt: now}
	} else {
		e.count++
		e.lastAt = now
		// Check if we just hit max attempts
		if e.count >= l.config.VerifyMaxAttempts && e.lockedAt.IsZero() {
			e.lockedAt = now
			lockedOut = true
		}
	}

	// Update IP entry
	e = l.verifyByIP[ipKey]
	if e == nil || now.Sub(e.firstAt) >= time.Hour {
		l.verifyByIP[ipKey] = &entry{count: 1, firstAt: now, lastAt: now}
	} else {
		e.count++
		e.lastAt = now
	}

	return lockedOut
}

// ResetVerifyAttempts clears verify attempt counter after successful verification.
func (l *Limiter) ResetVerifyAttempts(identifier string) {
	idKey := l.hashKey("verify:id:", normalizeIdentifier(identifier))
	l.mu.Lock()
	delete(l.verifyByID, idKey)
	l.mu.Unlock()
}

func (l *Limiter) recordSend(idKey, ipKey string, now time.Time) {
	// Update identifier entry
	e := l.sendByID[idKey]
	if e == nil || now.Sub(e.firstAt) >= time.Hour {
		l.sendByID[idKey] = &entry{count: 1, firstAt: now, lastAt: now}
	} else {
		e.count++
		e.lastAt = now
	}

	// Update IP entry
	e = l.sendByIP[ipKey]
	if e == nil || now.Sub(e.firstAt) >= time.Hour {
		l.sendByIP[ipKey] = &entry{count: 1, firstAt: now, lastAt: now}
	} else {
		e.count++
		e.lastAt = now
	}
}

func (l *Limiter) hashKey(prefix, value string) string {
	hash := sha256.Sum256([]byte(value))
	return prefix + hex.EncodeToString(hash[:8])
}

// normalizeIdentifier lowercases the identifier to prevent case-based bypass.
func normalizeIdentifier(identifier string) string {
	return strings.ToLower(strings.TrimSpace(identifier))
}

func (l *Limiter) startCleanup() {
	l.cleanupOnce.Do(func() {
		l.cleanupWg.Add(1)
		go func() {
			defer l.cleanupWg.Done()
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-l.cleanupCtx.Done():
					return
				case <-ticker.C:
					l.cleanup()
				}
			}
		}()
	})
}

func (l *Limiter) cleanup() {
	now := l.clock.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	// Clean send entries older than 1 hour
	for k, e := range l.sendByID {
		if now.Sub(e.lastAt) > time.Hour {
			delete(l.sendByID, k)
		}
	}
	for k, e := range l.sendByIP {
		if now.Sub(e.lastAt) > time.Hour {
			delete(l.sendByIP, k)
		}
	}

	// Clean verify entries older than lockout + 1 hour
	maxAge := l.config.VerifyLockout + time.Hour
	for k, e := range l.verifyByID {
		if now.Sub(e.lastAt) > maxAge {
			delete(l.verifyByID, k)
		}
	}
	for k, e := range l.verifyByIP {
		if now.Sub(e.lastAt) > time.Hour {
			delete(l.verifyByIP, k)
		}
	}
}

// GetClientIP extracts the client IP from a request.
// When trustProxy is true, uses the rightmost IP from X-Forwarded-For (added by your proxy).
// When trustProxy is false, ignores X-Forwarded-For entirely (prevents spoofing).
func GetClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// Use RIGHTMOST IP - this is the one your proxy added, not user-supplied
			parts := strings.Split(xff, ",")
			for i := len(parts) - 1; i >= 0; i-- {
				ip := strings.TrimSpace(parts[i])
				// Skip private/internal IPs to find the real client
				if ip != "" && !isPrivateIP(ip) {
					return ip
				}
			}
			// All IPs are private, use the last one
			return strings.TrimSpace(parts[len(parts)-1])
		}

		// Check X-Real-IP (set by nginx)
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}

	// Fall back to RemoteAddr (direct connection or untrusted proxy)
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr might not have a port (e.g., Unix socket or malformed)
		// Try to parse as IP directly, otherwise return as-is
		if parsed := net.ParseIP(r.RemoteAddr); parsed != nil {
			return r.RemoteAddr
		}
		// Last resort: strip anything after last colon that looks like a port
		if idx := strings.LastIndex(r.RemoteAddr, ":"); idx != -1 {
			candidate := r.RemoteAddr[:idx]
			if net.ParseIP(candidate) != nil {
				return candidate
			}
		}
		return r.RemoteAddr
	}
	return ip
}

// privateNetworks holds parsed CIDR ranges for private/reserved IPs.
// Parsed once at package init for efficiency.
var privateNetworks []*net.IPNet

func init() {
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"::1/128",
		"fc00::/7",
		"fe80::/10", // Link-local
	}
	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("invalid private CIDR: " + cidr)
		}
		privateNetworks = append(privateNetworks, network)
	}
}

// isPrivateIP checks if an IP is in a private/reserved range.
// Handles both IPv4 and IPv4-mapped IPv6 addresses (e.g., ::ffff:192.168.1.1).
func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// Convert IPv4-mapped IPv6 to IPv4 for consistent matching
	// e.g., ::ffff:192.168.1.1 -> 192.168.1.1
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}

	for _, network := range privateNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// SanitizeIdentifier masks an identifier for logging.
func SanitizeIdentifier(identifier string) string {
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	if strings.Contains(identifier, "@") {
		parts := strings.Split(identifier, "@")
		if len(parts[0]) > 2 {
			return parts[0][:2] + "***@" + parts[1]
		}
		return "***@" + parts[1]
	}
	// Phone: show last 4 digits
	if len(identifier) >= 4 {
		return "***" + identifier[len(identifier)-4:]
	}
	return "***"
}

// LogRateLimitExceeded logs a rate limit event with sanitized identifier.
func LogRateLimitExceeded(limitType, identifier, ip, reason string) {
	log.Warn().
		Str("event", "rate_limit_exceeded").
		Str("type", limitType).
		Str("identifier", SanitizeIdentifier(identifier)).
		Str("ip", ip).
		Str("reason", reason).
		Msg("OTP rate limit exceeded")
}
