package router

import (
	"net/http"
	"os"
	"path/filepath"
)

// Static registers a route that serves files from a local directory
// under the given URL prefix.
//
// Example:
//
//	r.Static("/static", "./public")
//	GET /static/js/app.js → ./public/js/app.js
func (r *Router) Static(prefix, dir string) {
	// Ensure prefix always starts with "/"
	if prefix == "" || prefix[0] != '/' {
		prefix = "/" + prefix
	}

	// Register a GET route with wildcard *filepath
	r.Get(prefix+"/*filepath", func(w http.ResponseWriter, req *http.Request, params map[string]string) {
		relPath := params["filepath"]

		// Clean the relative path to avoid directory traversal (e.g., "../../etc/passwd")
		cleanPath := filepath.Clean(relPath)

		// Construct the full path inside the given directory
		fullPath := filepath.Join(dir, cleanPath)

		// Check if the file exists
		if _, err := os.Stat(fullPath); err != nil {
			http.NotFound(w, req)
			return
		}

		// Serve the file
		http.ServeFile(w, req, fullPath)
	})
}
