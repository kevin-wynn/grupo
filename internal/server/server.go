package server

import (
	"context"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"sync"

	"github.com/kevin-wynn/grupo/internal/api"
	"github.com/kevin-wynn/grupo/internal/config"
	"github.com/kevin-wynn/grupo/internal/db"
	"github.com/kevin-wynn/grupo/internal/router"
	"github.com/kevin-wynn/grupo/internal/staticserve"
)

type Server struct {
	cfg     *config.Config
	store   *db.Store
	api     *api.Handler
	matcher *router.Matcher
	mu      sync.RWMutex
	projects map[string]db.Project
	webFS   fs.FS
	viteProxy *httputil.ReverseProxy
}

func New(cfg *config.Config, store *db.Store, apiHandler *api.Handler, webFS fs.FS) (*Server, error) {
	s := &Server{
		cfg:      cfg,
		store:    store,
		api:      apiHandler,
		matcher:  router.NewMatcher(cfg.AdminDomain),
		projects: map[string]db.Project{},
		webFS:    webFS,
	}
	if cfg.Dev && cfg.ViteDevURL != "" {
		target, err := url.Parse(cfg.ViteDevURL)
		if err != nil {
			return nil, err
		}
		s.viteProxy = httputil.NewSingleHostReverseProxy(target)
	}
	if err := s.reloadProjects(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) reloadProjects() error {
	projects, err := s.store.ListProjects(context.Background())
	if err != nil {
		return err
	}
	domains := make(map[string]string, len(projects))
	byID := make(map[string]db.Project, len(projects))
	for _, p := range projects {
		domains[p.Domain] = p.ID
		byID[p.ID] = p
	}
	s.mu.Lock()
	s.projects = byID
	s.matcher.SetProjects(domains)
	s.mu.Unlock()
	return nil
}

func (s *Server) ReloadRoutes() {
	if err := s.reloadProjects(); err != nil {
		log.Printf("reload routes: %v", err)
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.api.Register(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			mux.ServeHTTP(w, r)
			return
		}
		route := s.matcher.Match(r)
		switch route.Kind {
		case router.RouteAdmin:
			if s.handleAdmin(w, r, mux) {
				return
			}
		case router.RouteSite:
			s.handleSite(w, r, route.ProjectID)
			return
		default:
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	})
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request, mux *http.ServeMux) bool {
	if r.URL.Path == "/healthz" ||
		r.URL.Path == "/webhooks/github" ||
		stringsHasPrefix(r.URL.Path, "/auth/") ||
		stringsHasPrefix(r.URL.Path, "/api/") {
		mux.ServeHTTP(w, r)
		return true
	}
	if s.cfg.Dev && s.viteProxy != nil {
		s.viteProxy.ServeHTTP(w, r)
		return true
	}
	s.serveEmbeddedUI(w, r)
	return true
}

func (s *Server) serveEmbeddedUI(w http.ResponseWriter, r *http.Request) {
	if s.webFS == nil {
		http.NotFound(w, r)
		return
	}
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}
	f, err := s.webFS.Open(stringsTrimPrefix(path, "/"))
	if err != nil {
		f, err = s.webFS.Open("index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		f2, err2 := s.webFS.Open("index.html")
		if err2 != nil {
			http.NotFound(w, r)
			return
		}
		defer f2.Close()
		serveFile(w, r, f2)
		return
	}
	serveFile(w, r, f)
}

func serveFile(w http.ResponseWriter, r *http.Request, f fs.File) {
	stat, _ := f.Stat()
	if stat != nil && !stat.IsDir() {
		if rs, ok := f.(io.ReadSeeker); ok {
			http.ServeContent(w, r, stat.Name(), stat.ModTime(), rs)
			return
		}
	}
	http.NotFound(w, r)
}

func (s *Server) handleSite(w http.ResponseWriter, r *http.Request, projectID string) {
	s.mu.RLock()
	project, ok := s.projects[projectID]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	root := filepath.Join(s.cfg.DataDir, "sites", projectID, "current")
	handler := &staticserve.Handler{RootDir: root, SPAFallback: project.SPAFallback}
	handler.ServeHTTP(w, r)
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func stringsTrimPrefix(s, prefix string) string {
	if stringsHasPrefix(s, prefix) {
		return s[len(prefix):]
	}
	return s
}
