package ratelimit

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

// mockClock is a controllable clock for testing.
type mockClock struct {
	mu  sync.Mutex
	now time.Time
}

func newMockClock() *mockClock {
	return &mockClock{now: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)}
}

func (c *mockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *mockClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestCheckOTPSend_Cooldown(t *testing.T) {
	clock := newMockClock()
	limiter := New(&Config{
		SendCooldown:     60 * time.Second,
		SendMaxPerHour:   5,
		SendMaxIPPerHour: 20,
		Clock:            clock,
	})
	defer limiter.Close()

	identifier := "test@example.com"
	ip := "192.168.1.1"

	// First request should be allowed
	result := limiter.CheckOTPSend(identifier, ip)
	if !result.Allowed {
		t.Errorf("First request should be allowed, got blocked: %s", result.Reason)
	}
	limiter.RecordOTPSend(identifier, ip)

	// Second request within cooldown should be blocked
	clock.Advance(30 * time.Second)
	result = limiter.CheckOTPSend(identifier, ip)
	if result.Allowed {
		t.Error("Second request within cooldown should be blocked")
	}
	if result.Reason != "cooldown" {
		t.Errorf("Expected reason 'cooldown', got '%s'", result.Reason)
	}
	if result.RetryAfter != 30*time.Second {
		t.Errorf("Expected RetryAfter 30s, got %v", result.RetryAfter)
	}

	// After cooldown expires, should be allowed
	clock.Advance(31 * time.Second)
	result = limiter.CheckOTPSend(identifier, ip)
	if !result.Allowed {
		t.Errorf("Request after cooldown should be allowed, got blocked: %s", result.Reason)
	}
}

func TestCheckOTPSend_HourlyLimit(t *testing.T) {
	clock := newMockClock()
	limiter := New(&Config{
		SendCooldown:     1 * time.Millisecond,
		SendMaxPerHour:   3,
		SendMaxIPPerHour: 20,
		Clock:            clock,
	})
	defer limiter.Close()

	identifier := "hourly@example.com"
	ip := "192.168.1.2"

	// First 3 requests should be allowed
	for i := 0; i < 3; i++ {
		clock.Advance(1 * time.Second)
		result := limiter.CheckOTPSend(identifier, ip)
		if !result.Allowed {
			t.Errorf("Request %d should be allowed, got blocked: %s", i+1, result.Reason)
		}
		limiter.RecordOTPSend(identifier, ip)
	}

	// 4th request should be blocked (hourly limit)
	clock.Advance(1 * time.Second)
	result := limiter.CheckOTPSend(identifier, ip)
	if result.Allowed {
		t.Error("4th request should be blocked (hourly limit)")
	}
	if result.Reason != "hourly_limit" {
		t.Errorf("Expected reason 'hourly_limit', got '%s'", result.Reason)
	}

	// After hour passes, should be allowed again
	clock.Advance(1 * time.Hour)
	result = limiter.CheckOTPSend(identifier, ip)
	if !result.Allowed {
		t.Errorf("Request after hour should be allowed, got blocked: %s", result.Reason)
	}
}

func TestCheckOTPSend_IPLimit(t *testing.T) {
	clock := newMockClock()
	limiter := New(&Config{
		SendCooldown:     1 * time.Millisecond,
		SendMaxPerHour:   100,
		SendMaxIPPerHour: 2,
		Clock:            clock,
	})
	defer limiter.Close()

	ip := "192.168.1.3"

	// First 2 requests from different identifiers should be allowed
	for i := 0; i < 2; i++ {
		identifier := "user" + string(rune('a'+i)) + "@example.com"
		clock.Advance(1 * time.Second)
		result := limiter.CheckOTPSend(identifier, ip)
		if !result.Allowed {
			t.Errorf("Request %d should be allowed, got blocked: %s", i+1, result.Reason)
		}
		limiter.RecordOTPSend(identifier, ip)
	}

	// 3rd request from same IP should be blocked
	clock.Advance(1 * time.Second)
	result := limiter.CheckOTPSend("userc@example.com", ip)
	if result.Allowed {
		t.Error("3rd request from same IP should be blocked")
	}
	if result.Reason != "ip_hourly_limit" {
		t.Errorf("Expected reason 'ip_hourly_limit', got '%s'", result.Reason)
	}
}

func TestCheckOTPSend_IdentifierNormalization(t *testing.T) {
	clock := newMockClock()
	limiter := New(&Config{
		SendCooldown:     60 * time.Second,
		SendMaxPerHour:   5,
		SendMaxIPPerHour: 20,
		Clock:            clock,
	})
	defer limiter.Close()

	ip := "192.168.1.1"

	// First request with lowercase
	result := limiter.CheckOTPSend("user@example.com", ip)
	if !result.Allowed {
		t.Error("First request should be allowed")
	}
	limiter.RecordOTPSend("user@example.com", ip)

	// Second request with UPPERCASE should be blocked (same identifier)
	result = limiter.CheckOTPSend("USER@EXAMPLE.COM", ip)
	if result.Allowed {
		t.Error("Request with different case should be blocked (same identifier)")
	}
	if result.Reason != "cooldown" {
		t.Errorf("Expected reason 'cooldown', got '%s'", result.Reason)
	}

	// Mixed case should also be blocked
	result = limiter.CheckOTPSend("User@Example.Com", ip)
	if result.Allowed {
		t.Error("Request with mixed case should be blocked")
	}
}

func TestCheckOTPVerify_MaxAttempts(t *testing.T) {
	clock := newMockClock()
	limiter := New(&Config{
		VerifyMaxAttempts:  3,
		VerifyLockout:      5 * time.Minute,
		VerifyMaxIPPerHour: 30,
		Clock:              clock,
	})
	defer limiter.Close()

	identifier := "verify@example.com"
	ip := "192.168.1.4"

	// First 3 attempts should be allowed, recording each
	for i := 0; i < 3; i++ {
		result := limiter.CheckOTPVerify(identifier, ip)
		if !result.Allowed {
			t.Errorf("Attempt %d should be allowed, got blocked: %s", i+1, result.Reason)
		}
		lockedOut := limiter.RecordOTPVerify(identifier, ip)
		if i < 2 && lockedOut {
			t.Errorf("Attempt %d should not trigger lockout", i+1)
		}
		if i == 2 && !lockedOut {
			t.Error("3rd attempt should trigger lockout")
		}
	}

	// 4th attempt should be blocked (locked out after 3rd attempt triggered lockout)
	result := limiter.CheckOTPVerify(identifier, ip)
	if result.Allowed {
		t.Error("4th attempt should be blocked (lockout)")
	}
	if result.Reason != "lockout" {
		t.Errorf("Expected reason 'lockout', got '%s'", result.Reason)
	}
	if result.RetryAfter != 5*time.Minute {
		t.Errorf("Expected RetryAfter 5m, got %v", result.RetryAfter)
	}

	// After lockout expires, should be allowed
	clock.Advance(5*time.Minute + 1*time.Second)
	result = limiter.CheckOTPVerify(identifier, ip)
	if !result.Allowed {
		t.Errorf("Attempt after lockout should be allowed, got blocked: %s", result.Reason)
	}
}

func TestCheckOTPVerify_ResetOnSuccess(t *testing.T) {
	clock := newMockClock()
	limiter := New(&Config{
		VerifyMaxAttempts:  3,
		VerifyLockout:      5 * time.Minute,
		VerifyMaxIPPerHour: 30,
		Clock:              clock,
	})
	defer limiter.Close()

	identifier := "reset@example.com"
	ip := "192.168.1.5"

	// Make 2 failed attempts
	for i := 0; i < 2; i++ {
		result := limiter.CheckOTPVerify(identifier, ip)
		if !result.Allowed {
			t.Errorf("Attempt %d should be allowed", i+1)
		}
		limiter.RecordOTPVerify(identifier, ip)
	}

	// Reset on successful verification
	limiter.ResetVerifyAttempts(identifier)

	// Should be able to make 3 more attempts
	for i := 0; i < 3; i++ {
		result := limiter.CheckOTPVerify(identifier, ip)
		if !result.Allowed {
			t.Errorf("Attempt %d after reset should be allowed, got blocked: %s", i+1, result.Reason)
		}
		limiter.RecordOTPVerify(identifier, ip)
	}

	// 4th should be blocked
	result := limiter.CheckOTPVerify(identifier, ip)
	if result.Allowed {
		t.Error("4th attempt after reset should be blocked")
	}
}

func TestCheckOTPVerify_IPLimit(t *testing.T) {
	clock := newMockClock()
	limiter := New(&Config{
		VerifyMaxAttempts:  100,
		VerifyLockout:      5 * time.Minute,
		VerifyMaxIPPerHour: 2,
		Clock:              clock,
	})
	defer limiter.Close()

	ip := "192.168.1.6"

	// First 2 requests from different identifiers should be allowed
	for i := 0; i < 2; i++ {
		identifier := "verifyip" + string(rune('a'+i)) + "@example.com"
		result := limiter.CheckOTPVerify(identifier, ip)
		if !result.Allowed {
			t.Errorf("Request %d should be allowed, got blocked: %s", i+1, result.Reason)
		}
		limiter.RecordOTPVerify(identifier, ip)
	}

	// 3rd request from same IP should be blocked
	result := limiter.CheckOTPVerify("verifyipc@example.com", ip)
	if result.Allowed {
		t.Error("3rd verify request from same IP should be blocked")
	}
	if result.Reason != "ip_hourly_limit" {
		t.Errorf("Expected reason 'ip_hourly_limit', got '%s'", result.Reason)
	}
}

func TestGetClientIP_TrustProxy(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		trustProxy bool
		expected   string
	}{
		{
			name:       "TrustProxy=true, XFF rightmost public IP",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50, 10.0.0.1"},
			remoteAddr: "10.0.0.1:12345",
			trustProxy: true,
			expected:   "203.0.113.50", // Rightmost non-private
		},
		{
			name:       "TrustProxy=true, XFF all private",
			headers:    map[string]string{"X-Forwarded-For": "192.168.1.1, 10.0.0.1"},
			remoteAddr: "10.0.0.1:12345",
			trustProxy: true,
			expected:   "10.0.0.1", // Last one when all private
		},
		{
			name:       "TrustProxy=true, X-Real-IP",
			headers:    map[string]string{"X-Real-IP": "203.0.113.51"},
			remoteAddr: "10.0.0.1:12345",
			trustProxy: true,
			expected:   "203.0.113.51",
		},
		{
			name:       "TrustProxy=false, ignores XFF",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.50"},
			remoteAddr: "192.168.1.100:54321",
			trustProxy: false,
			expected:   "192.168.1.100", // Uses RemoteAddr, ignores spoofed XFF
		},
		{
			name:       "TrustProxy=false, ignores X-Real-IP",
			headers:    map[string]string{"X-Real-IP": "203.0.113.51"},
			remoteAddr: "192.168.1.100:54321",
			trustProxy: false,
			expected:   "192.168.1.100",
		},
		{
			name:       "No headers, RemoteAddr only",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.100:54321",
			trustProxy: true,
			expected:   "192.168.1.100",
		},
		{
			name:       "RemoteAddr without port",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.100",
			trustProxy: false,
			expected:   "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}

			got := GetClientIP(r, tt.trustProxy)
			if got != tt.expected {
				t.Errorf("GetClientIP() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGetClientIP_SpoofingPrevention(t *testing.T) {
	// Attacker sends fake X-Forwarded-For header
	r, _ := http.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4") // Attacker-supplied
	r.RemoteAddr = "192.168.1.100:54321"       // Real connection

	// With TrustProxy=false, the fake header is ignored
	got := GetClientIP(r, false)
	if got != "192.168.1.100" {
		t.Errorf("Should ignore X-Forwarded-For when TrustProxy=false, got %q", got)
	}
}

func TestSanitizeIdentifier(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"john.doe@example.com", "jo***@example.com"},
		{"JOHN.DOE@EXAMPLE.COM", "jo***@example.com"}, // Normalized to lowercase
		{"ab@example.com", "***@example.com"},
		{"a@example.com", "***@example.com"},
		{"+15551234567", "***4567"},
		{"5551234567", "***4567"},
		{"123", "***"},
		{"", "***"},
		{"  User@Example.Com  ", "us***@example.com"}, // Trimmed and lowercased
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SanitizeIdentifier(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeIdentifier(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.SendCooldown != 60*time.Second {
		t.Errorf("SendCooldown = %v, want 60s", cfg.SendCooldown)
	}
	if cfg.SendMaxPerHour != 5 {
		t.Errorf("SendMaxPerHour = %d, want 5", cfg.SendMaxPerHour)
	}
	if cfg.SendMaxIPPerHour != 20 {
		t.Errorf("SendMaxIPPerHour = %d, want 20", cfg.SendMaxIPPerHour)
	}
	if cfg.VerifyMaxAttempts != 5 {
		t.Errorf("VerifyMaxAttempts = %d, want 5", cfg.VerifyMaxAttempts)
	}
	if cfg.VerifyLockout != 5*time.Minute {
		t.Errorf("VerifyLockout = %v, want 5m", cfg.VerifyLockout)
	}
	if cfg.VerifyMaxIPPerHour != 30 {
		t.Errorf("VerifyMaxIPPerHour = %d, want 30", cfg.VerifyMaxIPPerHour)
	}
}

func TestNew_NilConfig(t *testing.T) {
	limiter := New(nil)
	defer limiter.Close()

	if limiter == nil {
		t.Error("New(nil) should return a valid limiter")
	}
	if limiter.config.SendCooldown != 60*time.Second {
		t.Error("New(nil) should use default config")
	}
}

func TestLimiter_Close(t *testing.T) {
	limiter := New(nil)

	// Trigger cleanup goroutine
	limiter.CheckOTPSend("test@example.com", "1.2.3.4")

	// Close should not hang
	done := make(chan struct{})
	go func() {
		limiter.Close()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("Close() should not hang")
	}
}

func TestConcurrentAccess(t *testing.T) {
	clock := newMockClock()
	limiter := New(&Config{
		SendCooldown:       1 * time.Millisecond,
		SendMaxPerHour:     1000,
		SendMaxIPPerHour:   1000,
		VerifyMaxAttempts:  1000,
		VerifyLockout:      5 * time.Minute,
		VerifyMaxIPPerHour: 1000,
		Clock:              clock,
	})
	defer limiter.Close()

	var wg sync.WaitGroup
	numGoroutines := 100
	numOps := 100

	// Concurrent OTP sends
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			identifier := "user@example.com"
			ip := "192.168.1.1"
			for j := 0; j < numOps; j++ {
				result := limiter.CheckOTPSend(identifier, ip)
				if result.Allowed {
					limiter.RecordOTPSend(identifier, ip)
				}
			}
		}(i)
	}

	// Concurrent OTP verifies
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			identifier := "verify@example.com"
			ip := "192.168.1.2"
			for j := 0; j < numOps; j++ {
				result := limiter.CheckOTPVerify(identifier, ip)
				if result.Allowed {
					limiter.RecordOTPVerify(identifier, ip)
				}
			}
		}(i)
	}

	// Concurrent resets
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				limiter.ResetVerifyAttempts("verify@example.com")
			}
		}(i)
	}

	wg.Wait()
	// If we get here without race detector complaints, test passes
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		// IPv4 private ranges
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		{"192.168.255.255", true},
		{"127.0.0.1", true},
		// IPv6 private/reserved
		{"::1", true},
		{"fc00::1", true},
		{"fe80::1", true}, // Link-local
		// IPv4-mapped IPv6 addresses (must match their IPv4 equivalents)
		{"::ffff:10.0.0.1", true},
		{"::ffff:192.168.1.1", true},
		{"::ffff:172.16.0.1", true},
		{"::ffff:127.0.0.1", true},
		{"::ffff:8.8.8.8", false},   // Public IP in IPv4-mapped format
		{"::ffff:1.1.1.1", false},   // Public IP in IPv4-mapped format
		// Public IPs
		{"203.0.113.50", false},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"2001:4860:4860::8888", false}, // Google DNS IPv6
		// Invalid
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			got := isPrivateIP(tt.ip)
			if got != tt.expected {
				t.Errorf("isPrivateIP(%q) = %v, want %v", tt.ip, got, tt.expected)
			}
		})
	}
}

func TestCheckAndRecord_SeparateOps(t *testing.T) {
	// Verify that Check doesn't consume quota - only Record does
	clock := newMockClock()
	limiter := New(&Config{
		SendCooldown:     60 * time.Second,
		SendMaxPerHour:   1,
		SendMaxIPPerHour: 100,
		Clock:            clock,
	})
	defer limiter.Close()

	identifier := "test@example.com"
	ip := "192.168.1.1"

	// Multiple checks should all be allowed (no recording)
	for i := 0; i < 10; i++ {
		result := limiter.CheckOTPSend(identifier, ip)
		if !result.Allowed {
			t.Errorf("Check %d should be allowed without prior Record", i+1)
		}
	}

	// Now record once
	limiter.RecordOTPSend(identifier, ip)

	// Next check should be blocked (cooldown)
	result := limiter.CheckOTPSend(identifier, ip)
	if result.Allowed {
		t.Error("Check after Record should be blocked")
	}
}
