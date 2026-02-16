package daemon

import (
	"embed"
	"net/http"
)

//go:embed ui/index.html
var uiFS embed.FS

func (d *Daemon) serveUI(w http.ResponseWriter, r *http.Request) {
	data, err := uiFS.ReadFile("ui/index.html")
	if err != nil {
		http.Error(w, "UI not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}
