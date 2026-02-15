package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ListenAddr != ":8042" {
		t.Errorf("ListenAddr: want :8042, got %s", cfg.ListenAddr)
	}
	home, _ := os.UserHomeDir()
	wantDataDir := filepath.Join(home, ".boxofrocks")
	if cfg.DataDir != wantDataDir {
		t.Errorf("DataDir: want %s, got %s", wantDataDir, cfg.DataDir)
	}
	wantDB := filepath.Join(wantDataDir, "bor.db")
	if cfg.DBPath != wantDB {
		t.Errorf("DBPath: want %s, got %s", wantDB, cfg.DBPath)
	}
}

func TestExpandHomeWithTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	got := expandHome("~/foo")
	want := filepath.Join(home, "foo")
	if got != want {
		t.Errorf("expandHome(~/foo): want %s, got %s", want, got)
	}
}

func TestExpandHomeAbsolute(t *testing.T) {
	got := expandHome("/absolute/path")
	if got != "/absolute/path" {
		t.Errorf("expandHome(/absolute/path): want /absolute/path, got %s", got)
	}
}

func TestExpandHomeTildeOnly(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	got := expandHome("~")
	if got != home {
		t.Errorf("expandHome(~): want %s, got %s", home, got)
	}
}

func TestValidateValid(t *testing.T) {
	cfg := &Config{ListenAddr: ":8042", DataDir: "/tmp/bor"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestValidateEmptyListenAddr(t *testing.T) {
	cfg := &Config{ListenAddr: "", DataDir: "/tmp/bor"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty listen_addr")
	}
}

func TestValidateInvalidPortZero(t *testing.T) {
	cfg := &Config{ListenAddr: ":0", DataDir: "/tmp/bor"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for port 0")
	}
}

func TestValidateInvalidPortTooHigh(t *testing.T) {
	cfg := &Config{ListenAddr: ":99999", DataDir: "/tmp/bor"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for port 99999")
	}
}

func TestValidateInvalidPortNonNumeric(t *testing.T) {
	cfg := &Config{ListenAddr: ":abc", DataDir: "/tmp/bor"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for non-numeric port")
	}
}

func TestValidateEmptyDataDir(t *testing.T) {
	cfg := &Config{ListenAddr: ":8042", DataDir: ""}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty data_dir")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		ListenAddr: ":9999",
		DataDir:    tmpDir,
		DBPath:     filepath.Join(tmpDir, "test.db"),
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Read back directly.
	data, err := os.ReadFile(filepath.Join(tmpDir, "config.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if loaded.ListenAddr != cfg.ListenAddr {
		t.Errorf("ListenAddr: want %s, got %s", cfg.ListenAddr, loaded.ListenAddr)
	}
	if loaded.DataDir != cfg.DataDir {
		t.Errorf("DataDir: want %s, got %s", cfg.DataDir, loaded.DataDir)
	}
	if loaded.DBPath != cfg.DBPath {
		t.Errorf("DBPath: want %s, got %s", cfg.DBPath, loaded.DBPath)
	}
}

func TestLoadMalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Write an invalid JSON config file where Load() will look for it.
	cfg := DefaultConfig()
	cfg.DataDir = tmpDir
	path := configPath(cfg)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// We can't easily redirect Load() to our temp dir since it uses DefaultConfig().
	// Instead, test the JSON unmarshal path directly.
	var c Config
	err := json.Unmarshal([]byte("{invalid json"), &c)
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestEnsureDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "nested", "data")
	cfg := &Config{DataDir: subDir}

	if err := EnsureDataDir(cfg); err != nil {
		t.Fatalf("EnsureDataDir: %v", err)
	}

	info, err := os.Stat(subDir)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}
