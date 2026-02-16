package cli

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/github"
)

func runInit(args []string, gf globalFlags) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	repoFlag := fs.String("repo", "", "Repository in owner/name format")
	offline := fs.Bool("offline", false, "Skip initial sync")
	socketFlag := fs.Bool("socket", false, "Enable Unix domain socket for sandbox agents")
	jsonFlag := fs.Bool("json", false, "Enable file-based queue for sandbox agents")

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Determine the repo.
	repo := *repoFlag
	if repo == "" {
		repo = resolveRepo(gf)
	}
	if repo == "" {
		return fmt.Errorf("could not determine repository; use --repo owner/name")
	}

	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid repo format %q, expected owner/name", repo)
	}

	client := newClient(gf)

	// Step 1: Ensure daemon is running. Auto-start in background if not.
	if _, err := client.Health(); err != nil {
		fmt.Println("Daemon not running. Starting in background...")
		if startErr := runDaemonBackground(gf); startErr != nil {
			return fmt.Errorf("auto-start daemon: %w\nStart it manually with: bor daemon start", startErr)
		}
		// Wait for daemon to be fully ready.
		if err := waitForDaemon(client, 10*time.Second); err != nil {
			return fmt.Errorf("daemon started but not responding: %w", err)
		}
	}

	// Step 2: Check auth and provide guidance if missing.
	if _, err := github.ResolveToken(); err != nil {
		fmt.Println("Warning: no GitHub token found. Sync with GitHub will be disabled.")
		fmt.Println("To enable sync, authenticate with one of:")
		fmt.Println("  bor login              Enter a token interactively")
		fmt.Println("  gh auth login          Use GitHub CLI")
		fmt.Println("  export GITHUB_TOKEN=.. Set environment variable")
		fmt.Println()
	}

	// Step 3: Register the repo via daemon API.
	repoBody := map[string]interface{}{
		"owner": parts[0],
		"name":  parts[1],
	}
	localPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	repoBody["local_path"] = localPath
	if *socketFlag {
		repoBody["socket"] = true
	}
	if *jsonFlag {
		repoBody["queue"] = true
	}

	alreadyRegistered := false
	resp, err := client.Do("POST", "/repos", repoBody)
	if err != nil {
		return fmt.Errorf("register repo: %w", err)
	}
	if err := decodeOrError(resp, nil); err != nil {
		if !strings.Contains(err.Error(), "409") {
			return fmt.Errorf("register repo: %w", err)
		}
		alreadyRegistered = true
		if gf.pretty {
			fmt.Printf("Repository %s already registered.\n", repo)
		}
		// Always update local_path; enable socket/queue if flags were requested.
		updateBody := map[string]interface{}{
			"local_path": localPath,
		}
		if *socketFlag {
			updateBody["socket_enabled"] = true
		}
		if *jsonFlag {
			updateBody["queue_enabled"] = true
		}
		if _, updateErr := client.UpdateRepo(repo, updateBody); updateErr != nil {
			return fmt.Errorf("update repo: %w", updateErr)
		}
		if gf.pretty && *socketFlag {
			fmt.Println("Unix socket enabled.")
		}
		if gf.pretty && *jsonFlag {
			fmt.Println("File queue enabled.")
		}
	} else {
		if gf.pretty {
			fmt.Printf("Repository %s registered.\n", repo)
			if *socketFlag {
				fmt.Println("Unix socket enabled.")
			}
			if *jsonFlag {
				fmt.Println("File queue enabled.")
			}
		}
	}

	// Step 4: Ensure .boxofrocks is in .gitignore when socket or queue is enabled.
	if *socketFlag || *jsonFlag {
		if err := ensureGitignore(localPath, ".boxofrocks"); err != nil {
			if err != errGitignoreExists {
				if gf.pretty {
					fmt.Printf("Warning: could not update .gitignore: %v\n", err)
				}
			}
		} else if gf.pretty {
			fmt.Println("Added .boxofrocks to .gitignore.")
		}
	}

	// Step 5: Trigger initial sync unless --offline.
	if !*offline {
		if err := client.ForceSync(repo); err != nil {
			// Non-fatal: sync might not be available.
			if gf.pretty {
				fmt.Printf("Warning: could not trigger sync: %v\n", err)
			}
		} else {
			if gf.pretty {
				fmt.Println("Initial sync triggered.")
			}
		}
	}

	// Step 6: Print result.
	if gf.pretty {
		fmt.Println()
		fmt.Println("Ready! Run 'bor list' to see issues.")
	} else {
		status := "initialized"
		if alreadyRegistered {
			status = "already_registered"
		}
		printJSON(map[string]string{
			"status": status,
			"repo":   repo,
		})
	}

	return nil
}

// ensureGitignore adds entry to the .gitignore in dir if it's not already present.
// Returns nil without printing if the entry already exists.
func ensureGitignore(dir, entry string) error {
	gitignorePath := filepath.Join(dir, ".gitignore")

	// Check if the entry already exists.
	if f, err := os.Open(gitignorePath); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) == entry {
				return errGitignoreExists
			}
		}
	}

	// Append the entry with a trailing newline.
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// If the file is non-empty and doesn't end with a newline, add one first.
	info, err := f.Stat()
	if err != nil {
		return err
	}
	prefix := ""
	if info.Size() > 0 {
		// Read the last byte to check for trailing newline.
		buf := make([]byte, 1)
		rf, _ := os.Open(gitignorePath)
		rf.Seek(-1, 2)
		rf.Read(buf)
		rf.Close()
		if buf[0] != '\n' {
			prefix = "\n"
		}
	}

	_, err = fmt.Fprintf(f, "%s%s\n", prefix, entry)
	return err
}

// errGitignoreExists is a sentinel indicating the entry already exists.
var errGitignoreExists = fmt.Errorf("already in .gitignore")
