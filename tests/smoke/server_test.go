//go:build smoke

package smoke

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/codr1/Pickleicious/internal/testutil"
)

func TestServerStartup(t *testing.T) {
	repoRoot := findRepoRoot(t)
	tempDir := t.TempDir()

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
  secret_key: "test-secret-key-for-smoke-tests-only"

database:
  driver: "sqlite"
  filename: "%s"

open_play:
  enforcement_interval: "*/5 * * * *"

features:
  enable_metrics: false
  enable_tracing: false
  enable_debug: true
`, port, port, filepath.ToSlash(filepath.Join(tempDir, "db", "smoke.db")))

	if err := os.WriteFile(configPath, []byte(configBody), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cmd := exec.Command(binPath)
	cmd.Dir = tempDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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

	healthURL := fmt.Sprintf("http://localhost:%d/health", port)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(10 * time.Second)

	for {
		select {
		case <-waitDone:
			t.Fatalf("server exited before health check: %v\nstdout:\n%s\nstderr:\n%s", waitErr, stdout.String(), stderr.String())
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

	select {
	case <-waitDone:
		t.Fatalf("server exited unexpectedly: %v\nstdout:\n%s\nstderr:\n%s", waitErr, stdout.String(), stderr.String())
	default:
	}
}

func reservePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve port: %v", err)
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatal("failed to locate repo root with go.mod")
	return ""
}

func TestMigrationsApplied(t *testing.T) {
	db := testutil.NewTestDB(t)

	expectedTables := []string{
		"organizations",
		"facilities",
		"operating_hours",
		"users",
		"user_billing",
		"user_photos",
		"staff",
		"courts",
		"reservation_types",
		"recurrence_rules",
		"reservations",
		"reservation_courts",
		"reservation_participants",
		"cognito_config",
	}

	for _, table := range expectedTables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name = ?",
			table,
		).Scan(&name)
		if err == sql.ErrNoRows {
			t.Fatalf("missing expected table %q after migrations", table)
		}
		if err != nil {
			t.Fatalf("query table %q existence: %v", table, err)
		}
	}
}

func TestForeignKeyIntegrity(t *testing.T) {
	db := testutil.NewTestDB(t)

	var foreignKeysEnabled int
	if err := db.QueryRow("PRAGMA foreign_keys;").Scan(&foreignKeysEnabled); err != nil {
		t.Fatalf("query foreign_keys pragma: %v", err)
	}
	if foreignKeysEnabled != 1 {
		t.Fatalf("expected foreign_keys pragma enabled, got %d", foreignKeysEnabled)
	}

	_, err := db.Exec(
		`INSERT INTO facilities (organization_id, name, slug, timezone)
		 VALUES (9999, 'Bad Facility', 'bad-facility', 'UTC')`,
	)
	if err == nil {
		t.Fatal("expected foreign key constraint failure for invalid organization_id")
	}
}
