//go:build smoke

package smoke

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/codr1/Pickleicious/internal/db"
)

func TestDashboardSmoke(t *testing.T) {
	repoRoot := findRepoRoot(t)
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "db", "smoke.db")

	seedDashboardDB(t, dbPath)

	binPath := filepath.Join(tempDir, "pickleicious-server")
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/server")
	buildCmd.Dir = repoRoot
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build server: %v\n%s", err, buildOutput)
	}

	port := reservePort(t)
	configPath := filepath.Join(tempDir, "config.yaml")
	configBody := fmt.Sprintf(`app:
  name: "Pickleicious"
  environment: "development"
  port: %d
  base_url: "http://localhost:%d"
  base_domain: "example.com"

database:
  driver: "sqlite"
  filename: "%s"

open_play:
  enforcement_interval: "*/5 * * * *"

features:
  enable_metrics: false
  enable_tracing: false
  enable_debug: true
`, port, port, filepath.ToSlash(dbPath))

	if err := os.WriteFile(configPath, []byte(configBody), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cmd := exec.Command(binPath)
	cmd.Dir = tempDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "APP_SECRET_KEY=smoke-test-secret")

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	waitDone := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(waitDone)
	}()

	t.Cleanup(func() {
		if cmd.Process == nil {
			return
		}
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-waitDone:
			return
		case <-time.After(5 * time.Second):
		}
		_ = cmd.Process.Kill()
		select {
		case <-waitDone:
		case <-time.After(5 * time.Second):
			t.Logf("server process did not exit after kill")
		}
	})

	waitForHealth(t, port, &stdout, &stderr, waitDone, &waitErr)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("failed to create cookie jar: %v", err)
	}
	client := &http.Client{
		Timeout: 5 * time.Second,
		Jar:     jar,
	}

	loginURL := fmt.Sprintf("http://localhost:%d/api/v1/auth/staff-login", port)
	form := url.Values{
		"identifier": {"dev@test.local"},
		"password":   {"devpass"},
	}
	loginReq, err := http.NewRequest(http.MethodPost, loginURL, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("failed to build login request: %v", err)
	}
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	loginResp, err := client.Do(loginReq)
	if err != nil {
		t.Fatalf("staff login request failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	defer loginResp.Body.Close()
	io.Copy(io.Discard, loginResp.Body)
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("staff login status: got %d want %d\nstdout:\n%s\nstderr:\n%s", loginResp.StatusCode, http.StatusOK, stdout.String(), stderr.String())
	}

	assertDashboardResponse(t, client, fmt.Sprintf("http://localhost:%d/admin/dashboard", port), &stdout, &stderr)
	assertDashboardResponse(t, client, fmt.Sprintf("http://localhost:%d/api/v1/dashboard/metrics", port), &stdout, &stderr)

	select {
	case <-waitDone:
		t.Fatalf("server exited unexpectedly: %v\nstdout:\n%s\nstderr:\n%s", waitErr, stdout.String(), stderr.String())
	default:
	}
}

func seedDashboardDB(t *testing.T, dbPath string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatalf("failed to create db directory: %v", err)
	}

	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer database.Close()

	_, err = database.Exec(
		"INSERT INTO organizations (id, name, slug, status) VALUES (1, 'Smoke Org', 'smoke', 'active')",
	)
	if err != nil {
		t.Fatalf("failed to seed organization: %v", err)
	}

	_, err = database.Exec(
		"INSERT INTO facilities (id, organization_id, name, slug, timezone) VALUES (1, 1, 'Smoke Facility', 'smoke-facility', 'UTC')",
	)
	if err != nil {
		t.Fatalf("failed to seed facility: %v", err)
	}
}

func waitForHealth(t *testing.T, port int, stdout *bytes.Buffer, stderr *bytes.Buffer, waitDone <-chan struct{}, waitErr *error) {
	t.Helper()

	healthURL := fmt.Sprintf("http://localhost:%d/health", port)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(10 * time.Second)

	for {
		select {
		case <-waitDone:
			t.Fatalf("server exited before health check: %v\nstdout:\n%s\nstderr:\n%s", *waitErr, stdout.String(), stderr.String())
		default:
		}

		resp, err := client.Get(healthURL)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}

		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for health check\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func assertDashboardResponse(t *testing.T, client *http.Client, url string, stdout *bytes.Buffer, stderr *bytes.Buffer) {
	t.Helper()

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("dashboard request failed for %s: %v\nstdout:\n%s\nstderr:\n%s", url, err, stdout.String(), stderr.String())
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read dashboard response for %s: %v\nstdout:\n%s\nstderr:\n%s", url, err, stdout.String(), stderr.String())
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard status for %s: got %d want %d\nstdout:\n%s\nstderr:\n%s", url, resp.StatusCode, http.StatusOK, stdout.String(), stderr.String())
	}

	assertDashboardSections(t, string(body))
}

func assertDashboardSections(t *testing.T, body string) {
	t.Helper()

	expected := []string{
		"Utilization Rate",
		"Scheduled Reservations",
		"Bookings by Type",
		"Check-Ins",
		"Cancellation Rate",
		"Cancellation Impact",
	}

	for _, needle := range expected {
		if !strings.Contains(body, needle) {
			snippet := body
			if len(snippet) > 400 {
				snippet = snippet[:400]
			}
			t.Fatalf("dashboard response missing %q\nbody snippet:\n%s", needle, snippet)
		}
	}
}
