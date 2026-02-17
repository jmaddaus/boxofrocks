package cli

import (
	"fmt"
	"os"
	"strings"
)

const defaultHost = "http://127.0.0.1:8042"

const usage = `bor - Box of Rocks

Usage:
  bor [global flags] <command> [flags]

Commands:
  daemon     Manage the daemon (start, stop, status, logs)
  init       Initialize a repository
  login      Authenticate with GitHub
  logout     Remove stored GitHub token
  list       List issues
  create     Create an issue
  close      Close an issue
  comment    Add a comment to an issue
  update     Update an issue
  next       Get the next issue to work on
  assign     Assign an issue
  sync       Trigger a sync with GitHub
  config     Configure repo settings (trusted-authors-only)
  db         Database migration tools (version, check, downgrade)
  help       Show this help
  version    Show version

Global Flags:
  --host URL     Daemon URL (default: $TRACKER_HOST or http://127.0.0.1:8042)
  -r, --repo NAME  Repository owner/name (default: auto-detect from git remote)
  --pretty       Use pretty-printed output instead of JSON

Run 'bor <command> --help' for more information on a command.`

// globalFlags holds flags that are available to all subcommands.
type globalFlags struct {
	host    string
	repo    string
	pretty  bool
	version string
}

// parseGlobalFlags extracts global flags from the front of the argument list
// and returns the remaining args. Global flags must come before the subcommand.
func parseGlobalFlags(args []string) (globalFlags, []string) {
	gf := globalFlags{
		host: os.Getenv("TRACKER_HOST"),
	}
	if gf.host == "" {
		gf.host = defaultHost
	}

	remaining := args
	for len(remaining) > 0 {
		switch {
		case remaining[0] == "--pretty":
			gf.pretty = true
			remaining = remaining[1:]
		case remaining[0] == "--host" && len(remaining) > 1:
			gf.host = remaining[1]
			remaining = remaining[2:]
		case strings.HasPrefix(remaining[0], "--host="):
			gf.host = strings.TrimPrefix(remaining[0], "--host=")
			remaining = remaining[1:]
		case (remaining[0] == "--repo" || remaining[0] == "-r") && len(remaining) > 1:
			gf.repo = remaining[1]
			remaining = remaining[2:]
		case strings.HasPrefix(remaining[0], "--repo="):
			gf.repo = strings.TrimPrefix(remaining[0], "--repo=")
			remaining = remaining[1:]
		default:
			return gf, remaining
		}
	}

	return gf, remaining
}

// resolveRepo returns the repo from the global flag, or tries auto-detection.
// If neither works, it returns "" (the daemon will try to resolve it).
func resolveRepo(gf globalFlags) string {
	if gf.repo != "" {
		return gf.repo
	}
	if detected, err := detectRepo(); err == nil {
		return detected
	}
	return ""
}

// newClient creates a daemon HTTP client from the global flags.
func newClient(gf globalFlags) *Client {
	return NewClient(gf.host)
}

// Run dispatches the CLI based on the provided arguments.
func Run(args []string, version string) error {
	gf, remaining := parseGlobalFlags(args)
	gf.version = version

	if len(remaining) == 0 {
		fmt.Println(usage)
		return nil
	}

	cmd := remaining[0]
	subArgs := remaining[1:]

	switch cmd {
	case "help", "--help", "-h":
		fmt.Println(usage)
		return nil
	case "version", "--version", "-v":
		fmt.Printf("bor version %s\n", version)
		return nil
	case "daemon":
		return runDaemon(subArgs, gf)
	case "init":
		return runInit(subArgs, gf)
	case "login":
		return runLogin(subArgs, gf)
	case "logout":
		return runLogout(subArgs, gf)
	case "list":
		return runList(subArgs, gf)
	case "create":
		return runCreate(subArgs, gf)
	case "close":
		return runClose(subArgs, gf)
	case "comment":
		return runComment(subArgs, gf)
	case "update":
		return runUpdate(subArgs, gf)
	case "next":
		return runNext(subArgs, gf)
	case "assign":
		return runAssign(subArgs, gf)
	case "sync":
		return runSync(subArgs, gf)
	case "config":
		return runConfig(subArgs, gf)
	case "db":
		return runDB(subArgs, gf)
	default:
		return fmt.Errorf("unknown command: %s\nRun 'bor help' for usage", strings.TrimSpace(cmd))
	}
}
