package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kevin-wynn/grupo/internal/auth"
	"github.com/kevin-wynn/grupo/internal/build"
	"github.com/kevin-wynn/grupo/internal/config"
	"github.com/kevin-wynn/grupo/internal/db"
	gh "github.com/kevin-wynn/grupo/internal/github"
)

type Handler struct {
	cfg         *config.Config
	store       *db.Store
	worker      *build.Worker
	github      *gh.Client
	sessions    *auth.Manager
	reload      func()
	oauthStates sync.Map
}

func NewHandler(cfg *config.Config, store *db.Store, worker *build.Worker, ghClient *gh.Client, sessions *auth.Manager, reload func()) *Handler {
	return &Handler{
		cfg:      cfg,
		store:    store,
		worker:   worker,
		github:   ghClient,
		sessions: sessions,
		reload:   reload,
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("GET /auth/github", h.authGitHub)
	mux.HandleFunc("GET /auth/github/callback", h.authCallback)
	mux.HandleFunc("POST /auth/logout", h.authLogout)
	mux.HandleFunc("POST /webhooks/github", h.webhookGitHub)

	mux.HandleFunc("GET /api/me", h.requireAuth(h.me))
	mux.HandleFunc("GET /api/github/repos", h.requireAuth(h.listRepos))
	mux.HandleFunc("GET /api/projects", h.requireAuth(h.listProjects))
	mux.HandleFunc("POST /api/projects", h.requireAuth(h.createProject))
	mux.HandleFunc("GET /api/projects/{id}", h.requireAuth(h.getProject))
	mux.HandleFunc("PATCH /api/projects/{id}", h.requireAuth(h.updateProject))
	mux.HandleFunc("DELETE /api/projects/{id}", h.requireAuth(h.deleteProject))
	mux.HandleFunc("POST /api/projects/{id}/deploy", h.requireAuth(h.deployProject))
	mux.HandleFunc("GET /api/projects/{id}/deployments", h.requireAuth(h.listDeployments))
	mux.HandleFunc("GET /api/deployments/{id}", h.requireAuth(h.getDeployment))
	mux.HandleFunc("GET /api/deployments/{id}/logs", h.requireAuth(h.deploymentLogs))
	mux.HandleFunc("GET /api/settings", h.requireAuth(h.settings))
}

func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.cfg.SkipGitHubAuth {
			next(w, r)
			return
		}
		if _, err := h.sessions.UserFromRequest(r.Context(), r); err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	if h.cfg.SkipGitHubAuth {
		installs, _ := h.store.ListInstallations(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{
			"github_login":   "dev",
			"github_user_id": 0,
			"installations":  installs,
			"dev_mode":       true,
		})
		return
	}
	sess, err := h.sessions.UserFromRequest(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	installs, _ := h.store.ListInstallations(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"github_login":   sess.GitHubLogin,
		"github_user_id": sess.GitHubUserID,
		"installations":  installs,
	})
}

func (h *Handler) listRepos(w http.ResponseWriter, r *http.Request) {
	if h.github == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	idStr := r.URL.Query().Get("installation_id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "installation_id required")
		return
	}
	var installationID int64
	fmt.Sscan(idStr, &installationID)
	repos, err := h.github.ListRepos(r.Context(), installationID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, repos)
}

type projectInput struct {
	Name           string `json:"name"`
	InstallationID int64  `json:"installation_id"`
	GitHubOwner    string `json:"github_owner"`
	GitHubRepo     string `json:"github_repo"`
	Branch         string `json:"branch"`
	BuildImage     string `json:"build_image"`
	BuildCommand   string `json:"build_command"`
	OutputDir      string `json:"output_dir"`
	RootDir        string `json:"root_dir"`
	Domain         string `json:"domain"`
	SPAFallback    bool   `json:"spa_fallback"`
	EnvJSON        string `json:"env_json"`
	FixturePath    string `json:"fixture_path"`
}

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.store.ListProjects(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type projectView struct {
		db.Project
		LastDeployment *db.Deployment `json:"last_deployment,omitempty"`
	}
	out := make([]projectView, 0, len(projects))
	for _, p := range projects {
		view := projectView{Project: p}
		if d, err := h.store.LatestDeployment(r.Context(), p.ID); err == nil {
			view.LastDeployment = d
		}
		out = append(out, view)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	var in projectInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := h.projectFromInput(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !h.cfg.SkipGitHubAuth && h.github != nil && p.FixturePath == "" {
		webhookURL := fmt.Sprintf("https://%s/webhooks/github", h.cfg.AdminDomain)
		hookID, err := h.github.CreateWebhook(r.Context(), p.InstallationID, p.GitHubOwner, p.GitHubRepo, webhookURL, h.cfg.GitHub.WebhookSecret)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		p.WebhookID = hookID
	}
	if err := h.store.CreateProject(r.Context(), p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.reloadRoutes()
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handler) getProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.store.GetProject(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) updateProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := h.store.GetProject(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	var in projectInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.projectFromInput(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated.ID = existing.ID
	updated.WebhookID = existing.WebhookID
	updated.CreatedAt = existing.CreatedAt
	if err := h.store.UpdateProject(r.Context(), *updated); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.reloadRoutes()
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deleteProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.store.GetProject(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if h.github != nil && p.WebhookID > 0 {
		_ = h.github.DeleteWebhook(r.Context(), p.InstallationID, p.GitHubOwner, p.GitHubRepo, p.WebhookID)
	}
	if err := h.store.DeleteProject(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	siteDir := filepath.Join(h.cfg.DataDir, "sites", id)
	_ = os.RemoveAll(siteDir)
	repoDir := filepath.Join(h.cfg.DataDir, "repos", id)
	_ = os.RemoveAll(repoDir)
	h.reloadRoutes()
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) deployProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := h.store.GetProject(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	d, err := h.worker.Enqueue(r.Context(), build.Job{ProjectID: id, CommitMessage: "manual deploy"})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, d)
}

func (h *Handler) listDeployments(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	deployments, err := h.store.ListDeploymentsByProject(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, deployments)
}

func (h *Handler) getDeployment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, err := h.store.GetDeployment(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "deployment not found")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handler) deploymentLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, err := h.store.GetDeployment(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "deployment not found")
		return
	}
	if d.LogPath == "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		return
	}
	data, err := os.ReadFile(d.LogPath)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	githubConfigured := h.github != nil && h.cfg.GitHub.AppID != ""
	writeJSON(w, http.StatusOK, map[string]any{
		"admin_domain":      h.cfg.AdminDomain,
		"data_dir":          h.cfg.DataDir,
		"github_configured": githubConfigured,
		"skip_github_auth":  h.cfg.SkipGitHubAuth,
		"dev_mode":          h.cfg.Dev,
	})
}

func (h *Handler) authGitHub(w http.ResponseWriter, r *http.Request) {
	if h.github == nil {
		writeError(w, http.StatusServiceUnavailable, "github not configured")
		return
	}
	state, err := randomState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.oauthStates.Store(state, time.Now())
	redirectURL := fmt.Sprintf("https://%s/auth/github/callback", h.cfg.AdminDomain)
	if h.cfg.Dev {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		redirectURL = fmt.Sprintf("%s://%s/auth/github/callback", scheme, r.Host)
	}
	cfg := h.github.OAuthConfig(redirectURL)
	http.Redirect(w, r, cfg.AuthCodeURL(state), http.StatusFound)
}

func (h *Handler) authCallback(w http.ResponseWriter, r *http.Request) {
	if h.github == nil {
		writeError(w, http.StatusServiceUnavailable, "github not configured")
		return
	}
	state := r.URL.Query().Get("state")
	if _, ok := h.oauthStates.LoadAndDelete(state); !ok {
		writeError(w, http.StatusBadRequest, "invalid oauth state")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing code")
		return
	}
	redirectURL := fmt.Sprintf("https://%s/auth/github/callback", h.cfg.AdminDomain)
	if h.cfg.Dev {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		redirectURL = fmt.Sprintf("%s://%s/auth/github/callback", scheme, r.Host)
	}
	cfg := h.github.OAuthConfig(redirectURL)
	token, err := cfg.Exchange(r.Context(), code)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	user, err := h.github.FetchOAuthUser(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	sessToken, err := h.sessions.CreateSession(r.Context(), user.ID, user.Login)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.sessions.SetCookie(w, sessToken)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handler) authLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.SessionCookie); err == nil {
		_ = h.store.DeleteSession(r.Context(), cookie.Value)
	}
	h.sessions.ClearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) webhookGitHub(w http.ResponseWriter, r *http.Request) {
	body, err := gh.ReadWebhookBody(r, 1<<20)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sig := r.Header.Get("X-Hub-Signature-256")
	if !gh.VerifyWebhookSignature(h.cfg.GitHub.WebhookSecret, body, sig) {
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}
	event := r.Header.Get("X-GitHub-Event")
	switch event {
	case "push":
		h.handlePush(w, r, body)
	case "installation", "installation_repositories":
		h.handleInstallation(w, r, body)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

func (h *Handler) handlePush(w http.ResponseWriter, r *http.Request, body []byte) {
	evt, err := gh.ParsePushEvent(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	branch := gh.BranchFromRef(evt.Ref)
	owner, repo, ok := gh.SplitFullName(evt.Repository.FullName)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid repository")
		return
	}
	projects, err := h.store.ListProjects(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, p := range projects {
		if p.GitHubOwner == owner && p.GitHubRepo == repo && p.Branch == branch {
			_, _ = h.worker.Enqueue(r.Context(), build.Job{
				ProjectID:     p.ID,
				CommitSHA:     evt.HeadCommit.ID,
				CommitMessage: evt.HeadCommit.Message,
			})
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleInstallation(w http.ResponseWriter, r *http.Request, body []byte) {
	evt, err := gh.ParseInstallationEvent(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	switch evt.Action {
	case "created":
		_ = h.store.UpsertInstallation(r.Context(), db.Installation{
			ID:           evt.Installation.ID,
			AccountLogin: evt.Installation.Account.Login,
			AccountType:  evt.Installation.Account.Type,
			CreatedAt:    time.Now().UTC(),
		})
	case "deleted":
		_ = h.store.DeleteInstallation(r.Context(), evt.Installation.ID)
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) projectFromInput(in projectInput) (*db.Project, error) {
	if in.Name == "" {
		return nil, errors.New("name is required")
	}
	if in.Domain == "" {
		return nil, errors.New("domain is required")
	}
	if !config.ValidateDomain(in.Domain) {
		return nil, errors.New("invalid domain")
	}
	if in.BuildImage == "" {
		return nil, errors.New("build_image is required")
	}
	if in.BuildCommand == "" {
		return nil, errors.New("build_command is required")
	}
	if in.OutputDir == "" {
		in.OutputDir = "dist"
	}
	if in.Branch == "" {
		in.Branch = "main"
	}
	if in.EnvJSON == "" {
		in.EnvJSON = "{}"
	}
	return &db.Project{
		Name:           in.Name,
		InstallationID: in.InstallationID,
		GitHubOwner:    in.GitHubOwner,
		GitHubRepo:     in.GitHubRepo,
		Branch:         in.Branch,
		BuildImage:     in.BuildImage,
		BuildCommand:   in.BuildCommand,
		OutputDir:      in.OutputDir,
		RootDir:        in.RootDir,
		Domain:         in.Domain,
		SPAFallback:    in.SPAFallback,
		EnvJSON:        in.EnvJSON,
		FixturePath:    in.FixturePath,
	}, nil
}

func (h *Handler) reloadRoutes() {
	if h.reload != nil {
		h.reload()
	}
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SeedFixtureProject inserts the default fixture project if missing.
func SeedFixtureProject(ctx context.Context, store *db.Store, fixturePath string) (*db.Project, error) {
	projects, err := store.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if p.Domain == "app1.mygrupo.dev" {
			return &p, nil
		}
	}
	p := db.Project{
		Name:         "fixture-hello",
		Domain:       "app1.mygrupo.dev",
		BuildImage:   "alpine:3.20",
		BuildCommand: "mkdir -p dist && echo 'hello from fixture' > dist/index.html",
		OutputDir:    "dist",
		Branch:       "main",
		FixturePath:  fixturePath,
	}
	if err := store.CreateProject(ctx, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
