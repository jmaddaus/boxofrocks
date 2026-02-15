package cli

import (
	"flag"
	"fmt"
	"strings"
)

func runInit(args []string, gf globalFlags) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	repoFlag := fs.String("repo", "", "Repository in owner/name format")
	offline := fs.Bool("offline", false, "Skip initial sync")

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

	// Register the repo via daemon API.
	alreadyRegistered := false
	if err := client.CreateRepo(parts[0], parts[1]); err != nil {
		// If it already exists (409 conflict), that is acceptable.
		if !strings.Contains(err.Error(), "409") {
			return fmt.Errorf("register repo: %w", err)
		}
		alreadyRegistered = true
		if gf.pretty {
			fmt.Printf("Repository %s already registered.\n", repo)
		}
	} else {
		if gf.pretty {
			fmt.Printf("Repository %s registered.\n", repo)
		}
	}

	// Trigger initial sync unless --offline.
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

	if !gf.pretty {
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
