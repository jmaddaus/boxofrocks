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
	"syscall"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/config"
	"github.com/jmaddaus/boxofrocks/internal/store"
	"github.com/jmaddaus/boxofrocks/internal/sync"
)

// Daemon manages the HTTP server and its dependencies.
type Daemon struct {
	cfg       *config.Config
	store     store.Store
	syncMgr   *sync.SyncManager
	server    *http.Server
	startedAt time.Time
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
		cfg:   cfg,
		store: s,
	}

	mux := d.registerRoutes()
	handler := d.applyMiddleware(mux)

	d.server = &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return d, nil
}

// NewWithStore creates a Daemon with an injected store (useful for testing).
func NewWithStore(cfg *config.Config, s store.Store) *Daemon {
	return NewWithStoreAndSync(cfg, s, nil)
}

// NewWithStoreAndSync creates a Daemon with an injected store and optional SyncManager.
// This is used by the CLI daemon start command to pass in a fully-wired SyncManager.
func NewWithStoreAndSync(cfg *config.Config, s store.Store, sm *sync.SyncManager) *Daemon {
	d := &Daemon{
		cfg:     cfg,
		store:   s,
		syncMgr: sm,
	}

	mux := d.registerRoutes()
	handler := d.applyMiddleware(mux)

	d.server = &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: handler,
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

// Run starts the HTTP server and blocks until a SIGINT or SIGTERM is received
// or the provided context is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	d.startedAt = time.Now()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		slog.Info("boxofrocks daemon listening", "addr", d.cfg.ListenAddr)
		if err := d.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
			var opErr *net.OpError
			if errors.As(err, &opErr) && errors.Is(opErr.Err, syscall.EADDRINUSE) {
				return fmt.Errorf("port %s already in use; is another daemon running?", d.cfg.ListenAddr)
			}
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

	if err := d.store.Close(); err != nil {
		if firstErr == nil {
			firstErr = fmt.Errorf("store close: %w", err)
		}
	}

	return firstErr
}
