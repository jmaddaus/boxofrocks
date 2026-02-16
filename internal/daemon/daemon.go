package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	stdsync "sync"
	"syscall"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/config"
	"github.com/jmaddaus/boxofrocks/internal/github"
	"github.com/jmaddaus/boxofrocks/internal/model"
	"github.com/jmaddaus/boxofrocks/internal/store"
	"github.com/jmaddaus/boxofrocks/internal/sync"
)

// contextKey is a private type for context keys in this package.
type contextKey string

// socketRepoIDKey is the context key for the repo ID resolved from a Unix socket connection.
const socketRepoIDKey contextKey = "socketRepoID"

// Daemon manages the HTTP server and its dependencies.
type Daemon struct {
	cfg       *config.Config
	store     store.Store
	ghClient  github.Client
	syncMgr   *sync.SyncManager
	server    *http.Server
	startedAt time.Time

	socketMu    stdsync.Mutex
	socketLns   map[string]net.Listener // sockPath → listener
	socketRepos map[string]int          // sockPath → repoID

	queueMu    stdsync.Mutex
	queueStops map[string]chan struct{} // queueDir → stop channel
	queueRepos map[string]int          // queueDir → repoID
}

// New creates a new Daemon, opening the SQLite store and setting up the HTTP server.
func New(cfg *config.Config) (*Daemon, error) {
	if err := config.EnsureDataDir(cfg); err != nil {
		return nil, fmt.Errorf("ensure data dir: %w", err)
	}

	s, err := store.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	d := &Daemon{
		cfg:         cfg,
		store:       s,
		socketLns:   make(map[string]net.Listener),
		socketRepos: make(map[string]int),
		queueStops:  make(map[string]chan struct{}),
		queueRepos:  make(map[string]int),
	}

	mux := d.registerRoutes()
	handler := d.applyMiddleware(mux)

	d.server = &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
		ConnContext:  d.connContext,
	}

	return d, nil
}

// NewWithStore creates a Daemon with an injected store (useful for testing).
func NewWithStore(cfg *config.Config, s store.Store) *Daemon {
	return NewWithStoreAndSync(cfg, s, nil)
}

// NewWithStoreAndSync creates a Daemon with an injected store and optional SyncManager.
// This is used by the CLI daemon start command to pass in a fully-wired SyncManager.
func NewWithStoreAndSync(cfg *config.Config, s store.Store, sm *sync.SyncManager, gh ...github.Client) *Daemon {
	d := &Daemon{
		cfg:         cfg,
		store:       s,
		syncMgr:     sm,
		socketLns:   make(map[string]net.Listener),
		socketRepos: make(map[string]int),
		queueStops:  make(map[string]chan struct{}),
		queueRepos:  make(map[string]int),
	}
	if len(gh) > 0 {
		d.ghClient = gh[0]
	}

	mux := d.registerRoutes()
	handler := d.applyMiddleware(mux)

	d.server = &http.Server{
		Addr:        cfg.ListenAddr,
		Handler:     handler,
		ConnContext: d.connContext,
	}

	return d
}

// Handler returns the HTTP handler (used for testing with httptest).
func (d *Daemon) Handler() http.Handler {
	return d.server.Handler
}

// StartedAt returns the time when the daemon was started via Run().
func (d *Daemon) StartedAt() time.Time {
	return d.startedAt
}

// PIDFilePath returns the path to the daemon PID file.
func PIDFilePath(cfg *config.Config) string {
	return filepath.Join(cfg.DataDir, "daemon.pid")
}

// LogFilePath returns the path to the daemon log file.
func LogFilePath(cfg *config.Config) string {
	return filepath.Join(cfg.DataDir, "daemon.log")
}

// ReadPIDFile reads the PID from the daemon PID file. Returns 0 if not found.
func ReadPIDFile(cfg *config.Config) (int, error) {
	data, err := os.ReadFile(PIDFilePath(cfg))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid PID file content: %w", err)
	}
	return pid, nil
}

// writePIDFile writes the current process PID to the PID file.
func writePIDFile(cfg *config.Config) error {
	return os.WriteFile(PIDFilePath(cfg), []byte(strconv.Itoa(os.Getpid())+"\n"), 0644)
}

// removePIDFile removes the PID file.
func removePIDFile(cfg *config.Config) {
	os.Remove(PIDFilePath(cfg))
}

// connContext is the http.Server.ConnContext hook. For Unix socket connections,
// it injects the associated repo ID into the request context so that
// resolveRepo can use it without an explicit ?repo= or X-Repo header.
func (d *Daemon) connContext(ctx context.Context, c net.Conn) context.Context {
	if addr, ok := c.LocalAddr().(*net.UnixAddr); ok {
		d.socketMu.Lock()
		repoID, exists := d.socketRepos[addr.Name]
		d.socketMu.Unlock()
		if exists {
			return context.WithValue(ctx, socketRepoIDKey, repoID)
		}
	}
	return ctx
}

// CreateSocketForRepo creates a Unix domain socket listener for the given repo.
// It is safe to call multiple times for the same repo.
func (d *Daemon) CreateSocketForRepo(repo *model.RepoConfig) error {
	sockPath := repo.SocketPath()
	if sockPath == "" {
		return nil
	}

	d.socketMu.Lock()
	defer d.socketMu.Unlock()

	if _, ok := d.socketLns[sockPath]; ok {
		return nil // already listening
	}

	// Ensure the .boxofrocks/ directory exists.
	sockDir := filepath.Dir(sockPath)
	if err := os.MkdirAll(sockDir, 0700); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}

	// Remove stale socket file if present.
	os.Remove(sockPath)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen unix %s: %w", sockPath, err)
	}

	// Set socket permissions to owner-only.
	if err := os.Chmod(sockPath, 0700); err != nil {
		ln.Close()
		os.Remove(sockPath)
		return fmt.Errorf("chmod socket: %w", err)
	}

	d.socketLns[sockPath] = ln
	d.socketRepos[sockPath] = repo.ID

	go func() {
		if err := d.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Warn("socket serve error", "path", sockPath, "error", err)
		}
	}()

	slog.Info("unix socket listening", "path", sockPath)
	return nil
}

// removeSocket closes and removes a single Unix domain socket.
func (d *Daemon) removeSocket(sockPath string) {
	d.socketMu.Lock()
	defer d.socketMu.Unlock()

	if ln, ok := d.socketLns[sockPath]; ok {
		ln.Close()
		delete(d.socketLns, sockPath)
	}
	delete(d.socketRepos, sockPath)
	os.Remove(sockPath)
}

// cleanupSockets removes all socket files from disk.
func (d *Daemon) cleanupSockets() {
	d.socketMu.Lock()
	defer d.socketMu.Unlock()

	for sockPath := range d.socketLns {
		os.Remove(sockPath)
	}
	d.socketLns = make(map[string]net.Listener)
	d.socketRepos = make(map[string]int)
}

// startRepoSockets iterates registered repos and creates sockets for those with SocketEnabled.
func (d *Daemon) startRepoSockets() {
	repos, err := d.store.ListRepos(context.Background())
	if err != nil {
		slog.Warn("could not list repos for socket setup", "error", err)
		return
	}
	for _, repo := range repos {
		if repo.SocketEnabled && repo.LocalPath != "" {
			if err := d.CreateSocketForRepo(repo); err != nil {
				slog.Warn("could not create socket for repo", "repo", repo.FullName(), "error", err)
			}
		}
	}
}

// Run starts the HTTP server and blocks until a SIGINT or SIGTERM is received
// or the provided context is cancelled. It uses split Listen/Serve so the PID
// file is written only after successful port bind.
func (d *Daemon) Run(ctx context.Context) error {
	d.startedAt = time.Now()

	// Bind the port first so we fail fast on EADDRINUSE.
	ln, err := net.Listen("tcp", d.cfg.ListenAddr)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) && errors.Is(opErr.Err, syscall.EADDRINUSE) {
			return fmt.Errorf("port %s already in use; is another daemon running?", d.cfg.ListenAddr)
		}
		return fmt.Errorf("listen: %w", err)
	}

	// Write PID file now that we've bound the port.
	if err := writePIDFile(d.cfg); err != nil {
		ln.Close()
		return fmt.Errorf("write PID file: %w", err)
	}
	defer removePIDFile(d.cfg)

	// Create Unix domain sockets for repos that have them enabled.
	d.startRepoSockets()
	defer d.cleanupSockets()

	// Start file-based queues for sandbox agent communication.
	d.startFileQueues()
	defer d.cleanupFileQueues()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		slog.Info("boxofrocks daemon listening", "addr", d.cfg.ListenAddr)
		if err := d.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		slog.Info("context cancelled, shutting down...")
	case sig := <-sigCh:
		slog.Info("received signal, shutting down...", "signal", sig)
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
	}

	return d.Shutdown(context.Background())
}

// Shutdown gracefully shuts down the HTTP server and closes the store.
func (d *Daemon) Shutdown(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var firstErr error

	if err := d.server.Shutdown(shutdownCtx); err != nil {
		firstErr = fmt.Errorf("server shutdown: %w", err)
	}

	// Remove socket files from disk (listeners already closed by server.Shutdown).
	d.cleanupSockets()

	// Stop file queue goroutines.
	d.cleanupFileQueues()

	if err := d.store.Close(); err != nil {
		if firstErr == nil {
			firstErr = fmt.Errorf("store close: %w", err)
		}
	}

	return firstErr
}
