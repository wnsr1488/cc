package webui

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

func DistDir() string {
	candidates := []string{"web/dist", "../web/dist", "../../web/dist"}
	for _, dir := range candidates {
		if info, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !info.IsDir() {
			return dir
		}
	}
	return ""
}

func Mount(r chi.Router, distDir string) {
	if distDir == "" {
		return
	}
	fileServer := http.FileServer(http.Dir(distDir))
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			http.NotFound(w, req)
			return
		}
		cleanPath := filepath.Clean(req.URL.Path)
		if cleanPath == "." || cleanPath == "/" {
			http.ServeFile(w, req, filepath.Join(distDir, "index.html"))
			return
		}
		target := filepath.Join(distDir, cleanPath)
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, req)
			return
		}
		http.ServeFile(w, req, filepath.Join(distDir, "index.html"))
	})
}
