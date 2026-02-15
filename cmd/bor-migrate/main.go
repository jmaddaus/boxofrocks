package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/jmaddaus/boxofrocks/internal/store"
	_ "modernc.org/sqlite"
)

func usage() {
	fmt.Fprintf(os.Stderr, `bor-migrate — boxofrocks database migration tool

Usage:
  bor-migrate version <db-path>             Show current DB schema version
  bor-migrate downgrade <db-path> <version> Downgrade DB to target version
  bor-migrate check <db-path>               Check if DB is compatible with this binary

The default database path is ~/.boxofrocks/bor.db

Examples:
  bor-migrate version ~/.boxofrocks/bor.db
  bor-migrate downgrade ~/.boxofrocks/bor.db 1
  bor-migrate check ~/.boxofrocks/bor.db
`)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 3 {
		usage()
	}

	command := os.Args[1]
	dbPath := os.Args[2]

	switch command {
	case "version":
		runVersion(dbPath)
	case "check":
		runCheck(dbPath)
	case "downgrade":
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "error: downgrade requires a target version\n")
			usage()
		}
		target, err := strconv.Atoi(os.Args[3])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid version number: %s\n", os.Args[3])
			os.Exit(1)
		}
		runDowngrade(dbPath, target)
	default:
		fmt.Fprintf(os.Stderr, "error: unknown command %q\n", command)
		usage()
	}
}

func runVersion(dbPath string) {
	db, err := store.OpenRawDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	version, err := store.ReadDBVersion(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("database: %s\n", dbPath)
	fmt.Printf("schema version: %d\n", version)
	fmt.Printf("binary supports: %d\n", store.DBSchemaVersion)
}

func runCheck(dbPath string) {
	db, err := store.OpenRawDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	version, err := store.ReadDBVersion(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("database: %s\n", dbPath)
	fmt.Printf("schema version: %d\n", version)
	fmt.Printf("binary supports: %d\n", store.DBSchemaVersion)

	if version > store.DBSchemaVersion {
		fmt.Printf("\nINCOMPATIBLE: database is newer than this binary.\n")
		fmt.Printf("Run: bor-migrate downgrade %s %d\n", dbPath, store.DBSchemaVersion)
		os.Exit(1)
	}

	fmt.Printf("\nOK: database is compatible.\n")
}

func runDowngrade(dbPath string, target int) {
	db, err := store.OpenRawDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	current, err := store.ReadDBVersion(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("database: %s\n", dbPath)
	fmt.Printf("current version: %d\n", current)
	fmt.Printf("target version: %d\n", target)

	if target >= current {
		fmt.Fprintf(os.Stderr, "error: target version %d must be less than current version %d\n", target, current)
		os.Exit(1)
	}

	if err := store.DowngradeDB(db, current, target); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("downgraded: %d → %d\n", current, target)
}
