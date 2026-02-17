package cli

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/config"
	"github.com/jmaddaus/boxofrocks/internal/daemon"
	"github.com/jmaddaus/boxofrocks/internal/github"
	"github.com/jmaddaus/boxofrocks/internal/store"
	"github.com/jmaddaus/boxofrocks/internal/sync"
)

func runDaemon(args []string, gf globalFlags) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: bor daemon <start|stop|status|logs>")
	}
	switch args[0] {
	case "start":
		return runDaemonStart(args[1:], gf)
	case "stop":
		return runDaemonStop(gf)
	case "status":
		return runDaemonStatus(gf)
	case "logs":
		return runDaemonLogs(args[1:])
	default:
		return fmt.Errorf("unknown daemon subcommand: %s\nUsage: bor daemon <start|stop|status|logs>", args[0])
	}
}

func runDaemonStart(args []string, gf globalFlags) error {
	fs := flag.NewFlagSet("daemon start", flag.ContinueOnError)
	foreground := fs.Bool("foreground", false, "Run in foreground (default: background)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *foreground {
		return runDaemonForeground(gf)
	}
	return runDaemonBackground(gf)
}

func runDaemonForeground(gf globalFlags) error {
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

	// 5. Create and run daemon (passing syncMgr and ghClient for use in handlers).
	d := daemon.NewWithStoreAndSyncVersion(cfg, st, syncMgr, gf.version, ghClient)
	return d.Run(context.Background())
}

func runDaemonBackground(gf globalFlags) error {
	// Check if already running by hitting health endpoint.
	client := newClient(gf)
	if _, err := client.Health(); err == nil {
		return fmt.Errorf("daemon is already running at %s", gf.host)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := config.EnsureDataDir(cfg); err != nil {
		return fmt.Errorf("ensure data dir: %w", err)
	}

	// Re-exec ourselves with --foreground.
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	logPath := daemon.LogFilePath(cfg)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.Command(executable, "daemon", "start", "--foreground")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("start daemon: %w", err)
	}
	logFile.Close()

	// Poll health endpoint to confirm the child started.
	if err := waitForDaemon(client, 5*time.Second); err != nil {
		// Child may have died — read tail of log.
		logTail, _ := readTailLines(logPath, 5)
		if logTail != "" {
			return fmt.Errorf("daemon failed to start. Log output:\n%s", logTail)
		}
		return fmt.Errorf("daemon failed to start within 5s")
	}

	fmt.Printf("Daemon started (PID %d)\n", cmd.Process.Pid)
	return nil
}

// waitForDaemon polls the health endpoint until it responds or the timeout expires.
func waitForDaemon(client *Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := client.Health(); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not respond within %s", timeout)
}

func runDaemonStop(gf globalFlags) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	pid, err := daemon.ReadPIDFile(cfg)
	if err != nil {
		return fmt.Errorf("read PID file: %w", err)
	}
	if pid == 0 {
		return fmt.Errorf("no PID file found; daemon may not be running")
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		// Process may already be gone — clean up PID file.
		os.Remove(daemon.PIDFilePath(cfg))
		return fmt.Errorf("send SIGTERM to PID %d: %w", pid, err)
	}

	// Poll until process exits (up to 10s).
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		// Signal 0 checks if process exists.
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			fmt.Printf("Daemon stopped (PID %d)\n", pid)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	return fmt.Errorf("daemon (PID %d) did not stop within 10s", pid)
}

func runDaemonStatus(gf globalFlags) error {
	client := newClient(gf)
	health, err := client.Health()
	if err != nil {
		return fmt.Errorf("daemon not running at %s; start with: bor daemon start", gf.host)
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

func runDaemonLogs(args []string) error {
	fs := flag.NewFlagSet("daemon logs", flag.ContinueOnError)
	follow := fs.Bool("f", false, "Follow log output")
	lines := fs.Int("n", 20, "Number of lines to show")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logPath := daemon.LogFilePath(cfg)
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("no log file found at %s", logPath)
	}

	tailArgs := []string{"-n", strconv.Itoa(*lines)}
	if *follow {
		tailArgs = append(tailArgs, "-f")
	}
	tailArgs = append(tailArgs, logPath)

	cmd := exec.Command("tail", tailArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// readTailLines reads the last n lines from a file.
func readTailLines(path string, n int) (string, error) {
	out, err := exec.Command("tail", "-n", strconv.Itoa(n), path).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
