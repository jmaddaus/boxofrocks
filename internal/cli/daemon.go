package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jmaddaus/boxofrocks/internal/config"
	"github.com/jmaddaus/boxofrocks/internal/daemon"
	"github.com/jmaddaus/boxofrocks/internal/github"
	"github.com/jmaddaus/boxofrocks/internal/store"
	"github.com/jmaddaus/boxofrocks/internal/sync"
)

func runDaemon(args []string, gf globalFlags) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: at daemon <start|status>")
	}
	switch args[0] {
	case "start":
		return runDaemonStart(gf)
	case "status":
		return runDaemonStatus(gf)
	default:
		return fmt.Errorf("unknown daemon subcommand: %s\nUsage: at daemon <start|status>", args[0])
	}
}

func runDaemonStart(gf globalFlags) error {
	// 1. Load config.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := config.EnsureDataDir(cfg); err != nil {
		return fmt.Errorf("ensure data dir: %w", err)
	}

	// 2. Open SQLite store.
	st, err := store.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	// 3. Resolve GitHub token (optional - warn if not found).
	token, tokenErr := github.ResolveToken()
	var ghClient github.Client
	if tokenErr == nil {
		ghClient = github.NewClient(token)
	} else {
		slog.Info("GitHub token not found, sync disabled", "error", tokenErr)
	}

	// 4. Create SyncManager (if we have a GitHub client).
	var syncMgr *sync.SyncManager
	if ghClient != nil {
		syncMgr = sync.NewSyncManager(st, ghClient)
		// Start syncers for all registered repos.
		repos, listErr := st.ListRepos(context.Background())
		if listErr != nil {
			slog.Error("could not list repos for sync", "error", listErr)
		} else {
			for _, repo := range repos {
				if addErr := syncMgr.AddRepo(repo); addErr != nil {
					slog.Error("could not start syncer", "repo", repo.FullName(), "error", addErr)
				}
			}
		}
		defer syncMgr.Stop()
	}

	// 5. Create and run daemon (passing syncMgr for use in handlers).
	d := daemon.NewWithStoreAndSync(cfg, st, syncMgr)
	return d.Run(context.Background())
}

func runDaemonStatus(gf globalFlags) error {
	client := newClient(gf)
	health, err := client.Health()
	if err != nil {
		return fmt.Errorf("daemon not running at %s; start with: at daemon start", gf.host)
	}

	if gf.pretty {
		status, _ := health["status"].(string)
		fmt.Printf("Daemon status: %s\n", status)

		if uptime, ok := health["uptime"].(string); ok {
			fmt.Printf("Uptime:        %s\n", uptime)
		}

		if repos, ok := health["repos"].([]interface{}); ok {
			fmt.Printf("Repos: %d registered\n", len(repos))
			for _, r := range repos {
				fmt.Printf("  - %v\n", r)
			}
		}

		if syncStatus, ok := health["sync_status"].(map[string]interface{}); ok && len(syncStatus) > 0 {
			fmt.Println("Sync status:")
			for repoName, info := range syncStatus {
				fmt.Printf("  %s:\n", repoName)
				if m, ok := info.(map[string]interface{}); ok {
					if lastSync, ok := m["last_sync"].(string); ok {
						fmt.Printf("    Last sync:      %s\n", lastSync)
					}
					if pending, ok := m["pending_events"].(float64); ok {
						fmt.Printf("    Pending events: %d\n", int(pending))
					}
					if lastErr, ok := m["last_error"].(string); ok && lastErr != "" {
						fmt.Printf("    Last error:     %s\n", lastErr)
					}
				}
			}
		}
	} else {
		printJSON(health)
	}
	return nil
}
