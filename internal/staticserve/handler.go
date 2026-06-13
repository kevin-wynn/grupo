package staticserve

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Handler struct {
	RootDir     string
	SPAFallback bool
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "" {
		http.NotFound(w, r)
		return
	}
	clean := filepath.Clean("/" + r.URL.Path)
	if strings.Contains(clean, "..") {
		http.NotFound(w, r)
		return
	}
	rel := strings.TrimPrefix(clean, "/")
	target := filepath.Join(h.RootDir, rel)

	info, err := os.Stat(target)
	if err == nil && info.IsDir() {
		index := filepath.Join(target, "index.html")
		if _, err := os.Stat(index); err == nil {
			http.ServeFile(w, r, index)
			return
		}
	}

	if err == nil && !info.IsDir() {
		http.ServeFile(w, r, target)
		return
	}

	if h.SPAFallback {
		index := filepath.Join(h.RootDir, "index.html")
		if _, err := os.Stat(index); err == nil {
			http.ServeFile(w, r, index)
			return
		}
	}

	http.NotFound(w, r)
}
