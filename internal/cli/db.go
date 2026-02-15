package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/jmaddaus/boxofrocks/internal/store"
	_ "modernc.org/sqlite"
)

const dbUsage = `Usage:
  bor db <command> <db-path> [args]

Commands:
  version   <db-path>             Show current DB schema version
  check     <db-path>             Check if DB is compatible with this binary
  downgrade <db-path> <version>   Downgrade DB to target version

Examples:
  bor db version ~/.boxofrocks/bor.db
  bor db downgrade ~/.boxofrocks/bor.db 1
  bor db check ~/.boxofrocks/bor.db`

func runDB(args []string, _ globalFlags) error {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, dbUsage)
		return fmt.Errorf("usage: bor db <command> <db-path>")
	}

	command := args[0]
	dbPath := args[1]

	switch command {
	case "version":
		return runDBVersion(dbPath)
	case "check":
		return runDBCheck(dbPath)
	case "downgrade":
		if len(args) < 3 {
			return fmt.Errorf("downgrade requires a target version\n%s", dbUsage)
		}
		target, err := strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("invalid version number: %s", args[2])
		}
		return runDBDowngrade(dbPath, target)
	default:
		return fmt.Errorf("unknown db subcommand: %s\n%s", command, dbUsage)
	}
}

func runDBVersion(dbPath string) error {
	db, err := store.OpenRawDB(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	version, err := store.ReadDBVersion(db)
	if err != nil {
		return fmt.Errorf("read version: %w", err)
	}

	fmt.Printf("database: %s\n", dbPath)
	fmt.Printf("schema version: %d\n", version)
	fmt.Printf("binary supports: %d\n", store.DBSchemaVersion)
	return nil
}

func runDBCheck(dbPath string) error {
	db, err := store.OpenRawDB(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	version, err := store.ReadDBVersion(db)
	if err != nil {
		return fmt.Errorf("read version: %w", err)
	}

	fmt.Printf("database: %s\n", dbPath)
	fmt.Printf("schema version: %d\n", version)
	fmt.Printf("binary supports: %d\n", store.DBSchemaVersion)

	if version > store.DBSchemaVersion {
		return fmt.Errorf("INCOMPATIBLE: database is newer than this binary.\nRun: bor db downgrade %s %d", dbPath, store.DBSchemaVersion)
	}

	fmt.Printf("\nOK: database is compatible.\n")
	return nil
}

func runDBDowngrade(dbPath string, target int) error {
	db, err := store.OpenRawDB(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	current, err := store.ReadDBVersion(db)
	if err != nil {
		return fmt.Errorf("read version: %w", err)
	}

	fmt.Printf("database: %s\n", dbPath)
	fmt.Printf("current version: %d\n", current)
	fmt.Printf("target version: %d\n", target)

	if target >= current {
		return fmt.Errorf("target version %d must be less than current version %d", target, current)
	}

	if err := store.DowngradeDB(db, current, target); err != nil {
		return fmt.Errorf("downgrade: %w", err)
	}

	fmt.Printf("downgraded: %d → %d\n", current, target)
	return nil
}
