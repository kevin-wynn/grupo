package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/kevin-wynn/grupo/internal/db"
	"golang.org/x/oauth2"
)

type OAuthUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

func (c *Client) FetchOAuthUser(ctx context.Context, token *oauth2.Token) (*OAuthUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github user: %s", string(body))
	}
	var user OAuthUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

// GitHub App user-to-server OAuth. Permissions are defined on the app — do not
// pass scopes in the authorize URL.
func (c *Client) OAuthConfig(redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://github.com/login/oauth/authorize",
			TokenURL: "https://github.com/login/oauth/access_token",
		},
		RedirectURL: redirectURL,
	}
}

func InstallURL(appSlug, state string) string {
	u := url.URL{
		Scheme: "https",
		Host:   "github.com",
		Path:   fmt.Sprintf("/apps/%s/installations/new", appSlug),
	}
	if state != "" {
		q := u.Query()
		q.Set("state", state)
		u.RawQuery = q.Encode()
	}
	return u.String()
}

type UserInstallation struct {
	ID      int64 `json:"id"`
	Account struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"account"`
}

func (c *Client) ListUserInstallations(ctx context.Context, userAccessToken string) ([]UserInstallation, error) {
	var all []UserInstallation
	page := 1
	for {
		u := fmt.Sprintf("https://api.github.com/user/installations?per_page=100&page=%d", page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+userAccessToken)
		req.Header.Set("Accept", "application/vnd.github+json")
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("list user installations: %s", string(body))
		}
		var out struct {
			Installations []UserInstallation `json:"installations"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Installations...)
		if len(out.Installations) < 100 {
			break
		}
		page++
	}
	return all, nil
}

func (c *Client) SyncUserInstallations(ctx context.Context, store *db.Store, userAccessToken string) error {
	installs, err := c.ListUserInstallations(ctx, userAccessToken)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, inst := range installs {
		if err := store.UpsertInstallation(ctx, db.Installation{
			ID:           inst.ID,
			AccountLogin: inst.Account.Login,
			AccountType:  inst.Account.Type,
			CreatedAt:    now,
		}); err != nil {
			return err
		}
	}
	return nil
}
