package github

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

type Client struct {
	AppID          string
	ClientID       string
	ClientSecret   string
	PrivateKeyPath string
	HTTPClient     *http.Client
	privateKey     *rsa.PrivateKey
}

type Repo struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
}

func NewClient(appID, clientID, clientSecret, privateKeyPath string) (*Client, error) {
	c := &Client{
		AppID:          appID,
		ClientID:       clientID,
		ClientSecret:   clientSecret,
		PrivateKeyPath: privateKeyPath,
		HTTPClient:     http.DefaultClient,
	}
	if privateKeyPath != "" {
		key, err := loadPrivateKey(privateKeyPath)
		if err != nil {
			return nil, err
		}
		c.privateKey = key
	}
	return c, nil
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("invalid pem")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		parsed, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		rsaKey, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not rsa")
		}
		return rsaKey, nil
	}
	return key, nil
}

func (c *Client) OAuthConfig(redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://github.com/login/oauth/authorize",
			TokenURL: "https://github.com/login/oauth/access_token",
		},
		RedirectURL: redirectURL,
		Scopes:      []string{"read:user"},
	}
}

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

func (c *Client) CreateAppJWT() (string, error) {
	if c.privateKey == nil {
		return "", fmt.Errorf("private key not loaded")
	}
	now := time.Now()
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now.Add(-time.Minute)),
		ExpiresAt: jwt.NewNumericDate(now.Add(9 * time.Minute)),
		Issuer:    c.AppID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(c.privateKey)
}

func (c *Client) InstallationToken(ctx context.Context, installationID int64) (string, error) {
	return c.installationToken(ctx, installationID)
}

func (c *Client) installationToken(ctx context.Context, installationID int64) (string, error) {
	jwtToken, err := c.CreateAppJWT()
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("installation token: %s", string(body))
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Token, nil
}

func (c *Client) ListRepos(ctx context.Context, installationID int64) ([]Repo, error) {
	token, err := c.installationToken(ctx, installationID)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("https://api.github.com/installation/repositories?per_page=100")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list repos: %s", string(body))
	}
	var out struct {
		Repositories []Repo `json:"repositories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Repositories, nil
}

func (c *Client) CreateWebhook(ctx context.Context, installationID int64, owner, repo, payloadURL, secret string) (int64, error) {
	token, err := c.installationToken(ctx, installationID)
	if err != nil {
		return 0, err
	}
	body := map[string]any{
		"name":   "web",
		"active": true,
		"events": []string{"push"},
		"config": map[string]string{
			"url":          payloadURL,
			"content_type": "json",
			"secret":       secret,
		},
	}
	data, _ := json.Marshal(body)
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/hooks", owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(data)))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("create webhook: %s", string(b))
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

func (c *Client) DeleteWebhook(ctx context.Context, installationID int64, owner, repo string, hookID int64) error {
	if hookID == 0 {
		return nil
	}
	token, err := c.installationToken(ctx, installationID)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/hooks/%d", owner, repo, hookID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete webhook: %s", string(b))
	}
	return nil
}

func CloneURLWithToken(owner, repo, token string) string {
	u := url.URL{
		Scheme: "https",
		Host:   "github.com",
		Path:   fmt.Sprintf("%s/%s.git", owner, repo),
	}
	if token != "" {
		u.User = url.UserPassword("x-access-token", token)
	}
	return u.String()
}

func ParseAppID(appID string) (int64, error) {
	return strconv.ParseInt(appID, 10, 64)
}
