package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmaddaus/boxofrocks/internal/model"
)

// ---------------------------------------------------------------------------
// File queue types
// ---------------------------------------------------------------------------

type fileQueueRequest struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body,omitempty"`
}

type fileQueueResponse struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

// queueResponseWriter captures the HTTP response for file queue dispatch.
type queueResponseWriter struct {
	statusCode int
	body       bytes.Buffer
	header     http.Header
}

func newQueueResponseWriter() *queueResponseWriter {
	return &queueResponseWriter{
		statusCode: http.StatusOK,
		header:     make(http.Header),
	}
}

func (w *queueResponseWriter) Header() http.Header {
	return w.header
}

func (w *queueResponseWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *queueResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// startFileQueues iterates registered repos and starts file queues for all local paths with QueueEnabled.
func (d *Daemon) startFileQueues() {
	repos, err := d.store.ListRepos(context.Background())
	if err != nil {
		slog.Warn("could not list repos for file queue setup", "error", err)
		return
	}
	for _, repo := range repos {
		for _, lp := range repo.LocalPaths {
			if qd := lp.QueueDir(); qd != "" {
				if err := d.startFileQueueAtPath(repo.ID, qd); err != nil {
					slog.Warn("could not start file queue", "repo", repo.FullName(), "path", lp.LocalPath, "error", err)
				}
			}
		}
	}
}

// startFileQueue creates the queue directory, cleans stale files, and starts
// a polling goroutine for the given repo. Safe to call multiple times.
func (d *Daemon) startFileQueue(repo *model.RepoConfig) error {
	queueDir := repo.QueueDir()
	if queueDir == "" {
		return nil
	}
	return d.startFileQueueAtPath(repo.ID, queueDir)
}

// startFileQueueAtPath creates the queue directory, cleans stale files, and starts
// a polling goroutine for the given path. Safe to call multiple times.
func (d *Daemon) startFileQueueAtPath(repoID int, queueDir string) error {
	if queueDir == "" {
		return nil
	}

	d.queueMu.Lock()
	defer d.queueMu.Unlock()

	if _, ok := d.queueStops[queueDir]; ok {
		return nil // already running
	}

	if err := os.MkdirAll(queueDir, 0700); err != nil {
		return err
	}

	cleanStaleQueueFiles(queueDir)
	writeBorAPIScript(queueDir)

	stop := make(chan struct{})
	d.queueStops[queueDir] = stop
	d.queueRepos[queueDir] = repoID

	go d.pollFileQueue(queueDir, repoID, stop)

	slog.Info("file queue started", "dir", queueDir)
	return nil
}

// stopFileQueue signals the polling goroutine to stop for the given queue dir.
func (d *Daemon) stopFileQueue(queueDir string) {
	d.queueMu.Lock()
	defer d.queueMu.Unlock()

	if ch, ok := d.queueStops[queueDir]; ok {
		close(ch)
		delete(d.queueStops, queueDir)
	}
	delete(d.queueRepos, queueDir)
}

// cleanupFileQueues stops all file queue goroutines.
func (d *Daemon) cleanupFileQueues() {
	d.queueMu.Lock()
	defer d.queueMu.Unlock()

	for _, ch := range d.queueStops {
		close(ch)
	}
	d.queueStops = make(map[string]chan struct{})
	d.queueRepos = make(map[string]int)
}

// ---------------------------------------------------------------------------
// Polling
// ---------------------------------------------------------------------------

func (d *Daemon) pollFileQueue(queueDir string, repoID int, stop chan struct{}) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			d.scanQueueDir(queueDir, repoID)
		}
	}
}

func (d *Daemon) scanQueueDir(queueDir string, repoID int) {
	entries, err := os.ReadDir(queueDir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("file queue readdir error", "dir", queueDir, "error", err)
		}
		return
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".req") {
			continue
		}
		// Skip .req.tmp files (partial writes).
		if strings.HasSuffix(name, ".req.tmp") {
			continue
		}
		reqPath := filepath.Join(queueDir, name)
		d.processQueueFile(reqPath, repoID)
	}
}

// ---------------------------------------------------------------------------
// Request processing
// ---------------------------------------------------------------------------

func (d *Daemon) processQueueFile(reqPath string, repoID int) {
	data, err := os.ReadFile(reqPath)
	if err != nil {
		slog.Warn("file queue read error", "path", reqPath, "error", err)
		return
	}

	var freq fileQueueRequest
	if err := json.Unmarshal(data, &freq); err != nil {
		// Write error response and clean up.
		d.writeQueueResponse(reqPath, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON in request file: " + err.Error(),
		})
		return
	}

	// Build a synthetic http.Request.
	var body *bytes.Reader
	if freq.Body != nil && string(freq.Body) != "null" {
		body = bytes.NewReader(freq.Body)
	} else {
		body = bytes.NewReader(nil)
	}

	httpReq, err := http.NewRequest(freq.Method, freq.Path, body)
	if err != nil {
		d.writeQueueResponse(reqPath, http.StatusBadRequest, map[string]string{
			"error": "invalid request: " + err.Error(),
		})
		return
	}

	if freq.Body != nil && string(freq.Body) != "null" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	// Inject repo ID via context, same key used by Unix socket connections.
	ctx := context.WithValue(httpReq.Context(), socketRepoIDKey, repoID)
	httpReq = httpReq.WithContext(ctx)

	// Dispatch through the existing handler chain.
	w := newQueueResponseWriter()
	d.server.Handler.ServeHTTP(w, httpReq)

	// Write the response atomically.
	d.writeQueueResponse(reqPath, w.statusCode, json.RawMessage(w.body.Bytes()))
}

// writeQueueResponse writes a response file atomically and removes the request file.
func (d *Daemon) writeQueueResponse(reqPath string, status int, body interface{}) {
	// Compute response path: replace .req with .resp
	base := strings.TrimSuffix(reqPath, ".req")
	respPath := base + ".resp"
	tmpPath := respPath + ".tmp"

	var rawBody json.RawMessage
	switch v := body.(type) {
	case json.RawMessage:
		rawBody = v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			slog.Warn("file queue marshal error", "error", err)
			return
		}
		rawBody = b
	}

	resp := fileQueueResponse{
		Status: status,
		Body:   rawBody,
	}

	respData, err := json.Marshal(resp)
	if err != nil {
		slog.Warn("file queue response marshal error", "error", err)
		return
	}

	if err := os.WriteFile(tmpPath, respData, 0600); err != nil {
		slog.Warn("file queue write tmp error", "path", tmpPath, "error", err)
		return
	}

	if err := os.Rename(tmpPath, respPath); err != nil {
		slog.Warn("file queue rename error", "from", tmpPath, "to", respPath, "error", err)
		os.Remove(tmpPath)
		return
	}

	// Remove the request file.
	os.Remove(reqPath)
}

// ---------------------------------------------------------------------------
// Helper script
// ---------------------------------------------------------------------------

const borAPIScript = `#!/usr/bin/env bash
# bor_api — file-based queue client for boxofrocks
# Usage: bor_api METHOD /path [json_body]

BOR_QUEUE="${BOR_QUEUE:-.boxofrocks/queue}"

bor_api() {
  local method="$1" path="$2" body="${3:-null}"
  local id
  id="$(date +%s%N)$$"
  local req="$BOR_QUEUE/${id}.req"
  local resp="$BOR_QUEUE/${id}.resp"

  mkdir -p "$BOR_QUEUE"

  printf '{"method":"%s","path":"%s","body":%s}\n' \
    "$method" "$path" "$body" > "${req}.tmp"
  mv "${req}.tmp" "$req"

  local i=0
  while [ ! -f "$resp" ] && [ $i -lt 300 ]; do
    sleep 0.1
    i=$((i + 1))
  done

  if [ -f "$resp" ]; then
    cat "$resp"
    rm -f "$req" "$resp"
  else
    echo '{"error":"timeout waiting for daemon response"}'
    rm -f "$req"
    return 1
  fi
}
`

// writeBorAPIScript writes .boxofrocks/bor_api.sh next to the queue directory.
// Overwrites on every start so the script stays current with the daemon version.
func writeBorAPIScript(queueDir string) {
	boxDir := filepath.Dir(queueDir) // .boxofrocks/queue → .boxofrocks
	scriptPath := filepath.Join(boxDir, "bor_api.sh")
	if err := os.WriteFile(scriptPath, []byte(borAPIScript), 0755); err != nil {
		slog.Warn("could not write bor_api.sh", "path", scriptPath, "error", err)
	}
}

// ---------------------------------------------------------------------------
// Stale file cleanup
// ---------------------------------------------------------------------------

// cleanStaleQueueFiles removes leftover .req, .resp, and .tmp files from a
// previous daemon run.
func cleanStaleQueueFiles(queueDir string) {
	entries, err := os.ReadDir(queueDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".req") || strings.HasSuffix(name, ".resp") || strings.HasSuffix(name, ".tmp") {
			os.Remove(filepath.Join(queueDir, name))
		}
	}
}
