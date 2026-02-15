package cli

import (
	"os"
	"testing"
)

func TestParseGlobalFlagsHost(t *testing.T) {
	os.Unsetenv("TRACKER_HOST")
	gf, remaining := parseGlobalFlags([]string{"--host", "http://x:1234", "list"})
	if gf.host != "http://x:1234" {
		t.Errorf("host: want http://x:1234, got %s", gf.host)
	}
	if len(remaining) != 1 || remaining[0] != "list" {
		t.Errorf("remaining: want [list], got %v", remaining)
	}
}

func TestParseGlobalFlagsRepo(t *testing.T) {
	os.Unsetenv("TRACKER_HOST")
	gf, remaining := parseGlobalFlags([]string{"--repo", "owner/name", "create"})
	if gf.repo != "owner/name" {
		t.Errorf("repo: want owner/name, got %s", gf.repo)
	}
	if len(remaining) != 1 || remaining[0] != "create" {
		t.Errorf("remaining: want [create], got %v", remaining)
	}
}

func TestParseGlobalFlagsPretty(t *testing.T) {
	os.Unsetenv("TRACKER_HOST")
	gf, remaining := parseGlobalFlags([]string{"--pretty", "list"})
	if !gf.pretty {
		t.Error("expected pretty=true")
	}
	if len(remaining) != 1 || remaining[0] != "list" {
		t.Errorf("remaining: want [list], got %v", remaining)
	}
}

func TestParseGlobalFlagsCombined(t *testing.T) {
	os.Unsetenv("TRACKER_HOST")
	gf, remaining := parseGlobalFlags([]string{"--host", "http://h:1", "--repo", "o/n", "--pretty", "cmd"})
	if gf.host != "http://h:1" {
		t.Errorf("host: want http://h:1, got %s", gf.host)
	}
	if gf.repo != "o/n" {
		t.Errorf("repo: want o/n, got %s", gf.repo)
	}
	if !gf.pretty {
		t.Error("expected pretty=true")
	}
	if len(remaining) != 1 || remaining[0] != "cmd" {
		t.Errorf("remaining: want [cmd], got %v", remaining)
	}
}

func TestParseGlobalFlagsNone(t *testing.T) {
	os.Unsetenv("TRACKER_HOST")
	gf, remaining := parseGlobalFlags([]string{"list"})
	if gf.host != defaultHost {
		t.Errorf("host: want %s, got %s", defaultHost, gf.host)
	}
	if gf.repo != "" {
		t.Errorf("repo: want empty, got %s", gf.repo)
	}
	if gf.pretty {
		t.Error("expected pretty=false")
	}
	if len(remaining) != 1 || remaining[0] != "list" {
		t.Errorf("remaining: want [list], got %v", remaining)
	}
}
