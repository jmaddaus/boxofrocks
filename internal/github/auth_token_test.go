package github

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokenFilePath(t *testing.T) {
	path, err := TokenFilePath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join(".boxofrocks", "token")) {
		t.Errorf("expected path to end with .boxofrocks/token, got %s", path)
	}
}

func TestSaveAndResolveFromTokenFile(t *testing.T) {
	// Use a temp dir as home to avoid touching real config.
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	// Save a token.
	if err := SaveToken("ghp_test_save_token_123"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	// Verify file permissions.
	path, _ := TokenFilePath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected 0600 permissions, got %04o", perm)
	}

	// Verify content.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "ghp_test_save_token_123" {
		t.Errorf("expected token 'ghp_test_save_token_123', got %q", got)
	}

	// Verify resolveFromTokenFile reads it back.
	token, err := resolveFromTokenFile()
	if err != nil {
		t.Fatalf("resolveFromTokenFile: %v", err)
	}
	if token != "ghp_test_save_token_123" {
		t.Errorf("expected 'ghp_test_save_token_123', got %q", token)
	}
}

func TestSaveToken_TrimsWhitespace(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	if err := SaveToken("  ghp_padded  \n"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	token, err := resolveFromTokenFile()
	if err != nil {
		t.Fatalf("resolveFromTokenFile: %v", err)
	}
	if token != "ghp_padded" {
		t.Errorf("expected trimmed token, got %q", token)
	}
}

func TestRemoveToken(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	// Save then remove.
	if err := SaveToken("ghp_to_remove"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	if err := RemoveToken(); err != nil {
		t.Fatalf("RemoveToken: %v", err)
	}

	// Verify file is gone.
	path, _ := TokenFilePath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected token file to be removed, but it still exists")
	}
}

func TestRemoveToken_NotExists(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	// Removing when no file exists should not error.
	if err := RemoveToken(); err != nil {
		t.Fatalf("RemoveToken on missing file: %v", err)
	}
}

func TestResolveFromTokenFile_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	// Write an empty token file.
	path, _ := TokenFilePath()
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("  \n"), 0600)

	_, err := resolveFromTokenFile()
	if err == nil {
		t.Error("expected error for empty token file")
	}
}

func TestResolveFromTokenFile_Missing(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	_, err := resolveFromTokenFile()
	if err == nil {
		t.Error("expected error when token file doesn't exist")
	}
}

func TestResolveToken_TokenFilePriority(t *testing.T) {
	// Token file should be checked before gh CLI and git credential,
	// but after GITHUB_TOKEN env var.
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	origToken := os.Getenv("GITHUB_TOKEN")
	os.Unsetenv("GITHUB_TOKEN")
	t.Cleanup(func() {
		if origToken != "" {
			os.Setenv("GITHUB_TOKEN", origToken)
		} else {
			os.Unsetenv("GITHUB_TOKEN")
		}
	})

	// Save a token to the file.
	if err := SaveToken("ghp_from_file"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	// ResolveToken should find it.
	token, err := ResolveToken()
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	if token != "ghp_from_file" {
		t.Errorf("expected 'ghp_from_file', got %q", token)
	}
}

func TestResolveToken_EnvOverridesTokenFile(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	origToken := os.Getenv("GITHUB_TOKEN")
	os.Setenv("GITHUB_TOKEN", "ghp_from_env")
	t.Cleanup(func() {
		if origToken != "" {
			os.Setenv("GITHUB_TOKEN", origToken)
		} else {
			os.Unsetenv("GITHUB_TOKEN")
		}
	})

	// Also save a token to file.
	if err := SaveToken("ghp_from_file"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	// Env var should win.
	token, err := ResolveToken()
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	if token != "ghp_from_env" {
		t.Errorf("expected 'ghp_from_env', got %q", token)
	}
}

func TestResolveTokenWithMethod_TokenFile(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	origToken := os.Getenv("GITHUB_TOKEN")
	os.Unsetenv("GITHUB_TOKEN")
	t.Cleanup(func() {
		if origToken != "" {
			os.Setenv("GITHUB_TOKEN", origToken)
		} else {
			os.Unsetenv("GITHUB_TOKEN")
		}
	})

	if err := SaveToken("ghp_method_test"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	methods, err := ResolveTokenWithMethod()
	if err != nil {
		t.Fatalf("ResolveTokenWithMethod: %v", err)
	}

	// Should have at least the token file method.
	found := false
	for _, m := range methods {
		if m.Name == "~/.boxofrocks/token" && m.Token == "ghp_method_test" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected token file method in results, got %+v", methods)
	}
}

func TestResolveTokenWithMethod_NoMethods(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	origToken := os.Getenv("GITHUB_TOKEN")
	os.Unsetenv("GITHUB_TOKEN")
	t.Cleanup(func() {
		if origToken != "" {
			os.Setenv("GITHUB_TOKEN", origToken)
		} else {
			os.Unsetenv("GITHUB_TOKEN")
		}
	})

	// With no token file and no env var, and likely no gh/git cred in test env.
	methods, err := ResolveTokenWithMethod()
	if err == nil && len(methods) > 0 {
		// gh or git credential might work in some test environments — that's ok.
		t.Skipf("found %d methods from other sources; cannot test empty path", len(methods))
	}
	if err == nil {
		t.Error("expected error when no methods available")
	}
}

func TestResolveToken_ErrorMentionsTokenFile(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	origToken := os.Getenv("GITHUB_TOKEN")
	os.Unsetenv("GITHUB_TOKEN")
	t.Cleanup(func() {
		if origToken != "" {
			os.Setenv("GITHUB_TOKEN", origToken)
		} else {
			os.Unsetenv("GITHUB_TOKEN")
		}
	})

	_, err := ResolveToken()
	if err == nil {
		t.Skip("token resolved from another source; cannot test error message")
	}

	msg := err.Error()
	if !strings.Contains(msg, "token file") {
		t.Errorf("error should mention token file, got: %s", msg)
	}
	if !strings.Contains(msg, "bor login") {
		t.Errorf("error should mention 'bor login', got: %s", msg)
	}
}
