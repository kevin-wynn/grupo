package api

import (
	"fmt"
	"net/http"

	gh "github.com/kevin-wynn/grupo/internal/github"
)

func (h *Handler) authRedirectURL(r *http.Request) string {
	if h.cfg.Dev {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		}
		return fmt.Sprintf("%s://%s/auth/github/callback", scheme, r.Host)
	}
	return fmt.Sprintf("https://%s/auth/github/callback", h.cfg.AdminDomain)
}

func (h *Handler) webhookPublicURL(r *http.Request) string {
	if h.cfg.Dev {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		}
		return fmt.Sprintf("%s://%s/webhooks/github", scheme, r.Host)
	}
	return fmt.Sprintf("https://%s/webhooks/github", h.cfg.AdminDomain)
}

func (h *Handler) authStatus(w http.ResponseWriter, r *http.Request) {
	loggedIn := false
	if !h.cfg.SkipGitHubAuth {
		if _, err := h.sessions.UserFromRequest(r.Context(), r); err == nil {
			loggedIn = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"skip_github_auth":  h.cfg.SkipGitHubAuth,
		"github_configured": h.github != nil && h.cfg.GitHub.AppID != "",
		"github_app_slug":   h.cfg.GitHub.AppSlug,
		"logged_in":         loggedIn || h.cfg.SkipGitHubAuth,
		"dev_mode":          h.cfg.Dev,
	})
}

func (h *Handler) authGitHub(w http.ResponseWriter, r *http.Request) {
	if h.github == nil {
		writeError(w, http.StatusServiceUnavailable, "github not configured")
		return
	}
	state, err := h.oauthState.Issue(w)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg := h.github.OAuthConfig(h.authRedirectURL(r))
	http.Redirect(w, r, cfg.AuthCodeURL(state), http.StatusFound)
}

func (h *Handler) authGitHubInstall(w http.ResponseWriter, r *http.Request) {
	if h.github == nil {
		writeError(w, http.StatusServiceUnavailable, "github not configured")
		return
	}
	if h.cfg.GitHub.AppSlug == "" {
		writeError(w, http.StatusBadRequest, "github app_slug is not configured")
		return
	}
	state, err := h.oauthState.Issue(w)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	http.Redirect(w, r, gh.InstallURL(h.cfg.GitHub.AppSlug, state), http.StatusFound)
}

func (h *Handler) authGitHubSetup(w http.ResponseWriter, r *http.Request) {
	// GitHub App "Setup URL" after installation — continue to user OAuth.
	http.Redirect(w, r, "/auth/github", http.StatusFound)
}

func (h *Handler) authCallback(w http.ResponseWriter, r *http.Request) {
	if h.github == nil {
		writeError(w, http.StatusServiceUnavailable, "github not configured")
		return
	}
	state := r.URL.Query().Get("state")
	if !h.oauthState.Verify(r, state) {
		http.Redirect(w, r, "/login?error=invalid_state", http.StatusFound)
		return
	}
	h.oauthState.Clear(w)

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, "/login?error=missing_code", http.StatusFound)
		return
	}

	cfg := h.github.OAuthConfig(h.authRedirectURL(r))
	token, err := cfg.Exchange(r.Context(), code)
	if err != nil {
		http.Redirect(w, r, "/login?error=oauth_exchange", http.StatusFound)
		return
	}
	user, err := h.github.FetchOAuthUser(r.Context(), token)
	if err != nil {
		http.Redirect(w, r, "/login?error=github_user", http.StatusFound)
		return
	}
	if err := h.github.SyncUserInstallations(r.Context(), h.store, token.AccessToken); err != nil {
		http.Redirect(w, r, "/login?error=sync_installations", http.StatusFound)
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
	if cookie, err := r.Cookie(h.sessions.CookieName()); err == nil {
		_ = h.store.DeleteSession(r.Context(), cookie.Value)
	}
	h.sessions.ClearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
