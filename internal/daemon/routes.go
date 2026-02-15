package daemon

import "net/http"

// registerRoutes sets up all API routes on a new ServeMux and returns it.
func (d *Daemon) registerRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// Health and sync.
	mux.HandleFunc("GET /health", d.health)
	mux.HandleFunc("POST /sync", d.forceSync)

	// Repos.
	mux.HandleFunc("POST /repos", d.addRepo)
	mux.HandleFunc("GET /repos", d.listRepos)

	// Issues: register /issues/next BEFORE /issues/{id} so the literal
	// route matches first.
	mux.HandleFunc("GET /issues/next", d.nextIssue)
	mux.HandleFunc("GET /issues/{id}", d.getIssue)
	mux.HandleFunc("GET /issues", d.listIssues)
	mux.HandleFunc("POST /issues", d.createIssue)
	mux.HandleFunc("PATCH /issues/{id}", d.updateIssue)
	mux.HandleFunc("DELETE /issues/{id}", d.deleteIssue)
	mux.HandleFunc("POST /issues/{id}/assign", d.assignIssue)

	return mux
}
