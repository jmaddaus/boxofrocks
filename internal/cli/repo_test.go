package cli

import "testing"

func TestParseGitRemoteURLHTTPS(t *testing.T) {
	got, err := parseGitRemoteURL("https://github.com/owner/name.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "owner/name" {
		t.Errorf("want owner/name, got %s", got)
	}
}

func TestParseGitRemoteURLHTTPSNoGit(t *testing.T) {
	got, err := parseGitRemoteURL("https://github.com/owner/name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "owner/name" {
		t.Errorf("want owner/name, got %s", got)
	}
}

func TestParseGitRemoteURLSSH(t *testing.T) {
	got, err := parseGitRemoteURL("git@github.com:owner/name.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "owner/name" {
		t.Errorf("want owner/name, got %s", got)
	}
}

func TestParseGitRemoteURLSSHNoGit(t *testing.T) {
	got, err := parseGitRemoteURL("git@github.com:owner/name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "owner/name" {
		t.Errorf("want owner/name, got %s", got)
	}
}

func TestParseGitRemoteURLHTTP(t *testing.T) {
	got, err := parseGitRemoteURL("http://github.com/owner/name.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "owner/name" {
		t.Errorf("want owner/name, got %s", got)
	}
}

func TestParseGitRemoteURLInvalid(t *testing.T) {
	_, err := parseGitRemoteURL("not-a-valid-url")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestParseGitRemoteURLEmpty(t *testing.T) {
	_, err := parseGitRemoteURL("")
	if err == nil {
		t.Error("expected error for empty string")
	}
}

func TestParseGitRemoteURLGitLab(t *testing.T) {
	got, err := parseGitRemoteURL("git@gitlab.com:owner/name.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "owner/name" {
		t.Errorf("want owner/name, got %s", got)
	}
}
