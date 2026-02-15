package cli

import (
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunDBVersionMemory(t *testing.T) {
	err := runDB([]string{"version", ":memory:"}, globalFlags{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDBCheckMemory(t *testing.T) {
	err := runDB([]string{"check", ":memory:"}, globalFlags{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDBNoArgs(t *testing.T) {
	err := runDB(nil, globalFlags{})
	if err == nil {
		t.Fatal("expected error for no args")
	}
}

func TestRunDBMissingDBPath(t *testing.T) {
	err := runDB([]string{"version"}, globalFlags{})
	if err == nil {
		t.Fatal("expected error for missing db path")
	}
}

func TestRunDBUnknownSubcommand(t *testing.T) {
	err := runDB([]string{"bogus", ":memory:"}, globalFlags{})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}

func TestRunDBDowngradeMissingVersion(t *testing.T) {
	err := runDB([]string{"downgrade", ":memory:"}, globalFlags{})
	if err == nil {
		t.Fatal("expected error for missing downgrade version")
	}
}

func TestRunDBDowngradeInvalidVersion(t *testing.T) {
	err := runDB([]string{"downgrade", ":memory:", "abc"}, globalFlags{})
	if err == nil {
		t.Fatal("expected error for invalid version number")
	}
}

func TestRunDBDowngradeTargetNotLess(t *testing.T) {
	// :memory: has version 0, so downgrading to 0 should fail (target >= current)
	err := runDB([]string{"downgrade", ":memory:", "0"}, globalFlags{})
	if err == nil {
		t.Fatal("expected error when target >= current")
	}
}
