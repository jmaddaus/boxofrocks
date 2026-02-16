package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jmaddaus/boxofrocks/internal/config"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	return &config.Config{
		ListenAddr: ":0",
		DataDir:    dir,
		DBPath:     filepath.Join(dir, "bor.db"),
	}
}

func TestPIDFilePath(t *testing.T) {
	cfg := testConfig(t)
	got := PIDFilePath(cfg)
	want := filepath.Join(cfg.DataDir, "daemon.pid")
	if got != want {
		t.Errorf("PIDFilePath = %q, want %q", got, want)
	}
}

func TestLogFilePath(t *testing.T) {
	cfg := testConfig(t)
	got := LogFilePath(cfg)
	want := filepath.Join(cfg.DataDir, "daemon.log")
	if got != want {
		t.Errorf("LogFilePath = %q, want %q", got, want)
	}
}

func TestWriteAndReadPIDFile(t *testing.T) {
	cfg := testConfig(t)

	// Write PID file with current process PID.
	if err := writePIDFile(cfg); err != nil {
		t.Fatalf("writePIDFile: %v", err)
	}

	// Read it back.
	pid, err := ReadPIDFile(cfg)
	if err != nil {
		t.Fatalf("ReadPIDFile: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("PID = %d, want %d", pid, os.Getpid())
	}
}

func TestReadPIDFile_NotExists(t *testing.T) {
	cfg := testConfig(t)

	pid, err := ReadPIDFile(cfg)
	if err != nil {
		t.Fatalf("ReadPIDFile: %v", err)
	}
	if pid != 0 {
		t.Errorf("expected PID 0 for missing file, got %d", pid)
	}
}

func TestReadPIDFile_InvalidContent(t *testing.T) {
	cfg := testConfig(t)

	// Write garbage.
	path := PIDFilePath(cfg)
	os.WriteFile(path, []byte("not-a-number\n"), 0644)

	_, err := ReadPIDFile(cfg)
	if err == nil {
		t.Error("expected error for invalid PID file content")
	}
	if !strings.Contains(err.Error(), "invalid PID") {
		t.Errorf("error should mention 'invalid PID', got: %v", err)
	}
}

func TestRemovePIDFile(t *testing.T) {
	cfg := testConfig(t)

	if err := writePIDFile(cfg); err != nil {
		t.Fatalf("writePIDFile: %v", err)
	}

	removePIDFile(cfg)

	if _, err := os.Stat(PIDFilePath(cfg)); !os.IsNotExist(err) {
		t.Error("expected PID file to be removed")
	}
}

func TestRemovePIDFile_NotExists(t *testing.T) {
	cfg := testConfig(t)

	// Should not panic or error.
	removePIDFile(cfg)
}

func TestWritePIDFile_Content(t *testing.T) {
	cfg := testConfig(t)

	if err := writePIDFile(cfg); err != nil {
		t.Fatalf("writePIDFile: %v", err)
	}

	data, err := os.ReadFile(PIDFilePath(cfg))
	if err != nil {
		t.Fatalf("read PID file: %v", err)
	}

	content := strings.TrimSpace(string(data))
	wantPID := strconv.Itoa(os.Getpid())
	if content != wantPID {
		t.Errorf("PID file content = %q, want %q", content, wantPID)
	}
}
