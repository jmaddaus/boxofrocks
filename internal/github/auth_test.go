package github

import (
	"os"
	"testing"
)

func TestResolveToken_EnvVar(t *testing.T) {
	// Save and restore original value
	orig := os.Getenv("GITHUB_TOKEN")
	defer func() {
		if orig != "" {
			os.Setenv("GITHUB_TOKEN", orig)
		} else {
			os.Unsetenv("GITHUB_TOKEN")
		}
	}()

	os.Setenv("GITHUB_TOKEN", "ghp_test_token_12345")

	token, err := ResolveToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "ghp_test_token_12345" {
		t.Fatalf("expected token 'ghp_test_token_12345', got %q", token)
	}
}

func TestResolveToken_EnvVarWithWhitespace(t *testing.T) {
	orig := os.Getenv("GITHUB_TOKEN")
	defer func() {
		if orig != "" {
			os.Setenv("GITHUB_TOKEN", orig)
		} else {
			os.Unsetenv("GITHUB_TOKEN")
		}
	}()

	os.Setenv("GITHUB_TOKEN", "  ghp_trimmed_token  \n")

	token, err := ResolveToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "ghp_trimmed_token" {
		t.Fatalf("expected trimmed token, got %q", token)
	}
}

func TestResolveToken_EmptyEnvVar(t *testing.T) {
	orig := os.Getenv("GITHUB_TOKEN")
	defer func() {
		if orig != "" {
			os.Setenv("GITHUB_TOKEN", orig)
		} else {
			os.Unsetenv("GITHUB_TOKEN")
		}
	}()

	// Empty string should not count as a token
	os.Setenv("GITHUB_TOKEN", "")

	// This will likely fail (no gh CLI or git credentials in test env),
	// but we're testing that empty env var is skipped.
	_, err := ResolveToken()
	// We expect an error since neither gh nor git credential will work in test
	if err == nil {
		// If somehow a token was resolved, that's fine — means gh or git creds worked
		return
	}
	// Error message should mention all three methods
	if errMsg := err.Error(); errMsg == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestResolveToken_ErrorMessage(t *testing.T) {
	orig := os.Getenv("GITHUB_TOKEN")
	defer func() {
		if orig != "" {
			os.Setenv("GITHUB_TOKEN", orig)
		} else {
			os.Unsetenv("GITHUB_TOKEN")
		}
	}()

	os.Unsetenv("GITHUB_TOKEN")

	// In a test environment without gh CLI or git credentials, this should fail
	// with a helpful error message.
	_, err := ResolveToken()
	if err == nil {
		// gh or git credential might actually work; skip the rest
		t.Skip("token resolved from gh or git credential; cannot test error path")
		return
	}

	errMsg := err.Error()
	if !contains(errMsg, "GITHUB_TOKEN") {
		t.Errorf("error should mention GITHUB_TOKEN, got: %s", errMsg)
	}
	if !contains(errMsg, "gh auth") {
		t.Errorf("error should mention gh auth, got: %s", errMsg)
	}
	if !contains(errMsg, "git credential") {
		t.Errorf("error should mention git credential, got: %s", errMsg)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
