package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type GitHubConfig struct {
	AppID          string `yaml:"app_id"`
	ClientID       string `yaml:"client_id"`
	ClientSecret   string `yaml:"client_secret"`
	WebhookSecret  string `yaml:"webhook_secret"`
	PrivateKeyPath string `yaml:"private_key_path"`
}

type Config struct {
	ListenAddr       string       `yaml:"listen_addr"`
	DataDir          string       `yaml:"data_dir"`
	AdminDomain      string       `yaml:"admin_domain"`
	SessionSecret    string       `yaml:"session_secret"`
	GitHub           GitHubConfig `yaml:"github"`
	Dev              bool         `yaml:"-"`
	SkipGitHubAuth   bool         `yaml:"-"`
	ViteDevURL       string       `yaml:"-"`
}

var domainPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$`)

func Load(path string) (*Config, error) {
	cfg := &Config{
		ListenAddr:  ":8080",
		DataDir:     "/var/lib/grupo",
		AdminDomain: "admin.mygrupo.dev",
	}

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read config: %w", err)
		}
		if err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parse config: %w", err)
			}
		}
	}

	applyEnv(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("GRUPO_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("GRUPO_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if cfg.DataDir != "" {
		if abs, err := filepath.Abs(cfg.DataDir); err == nil {
			cfg.DataDir = abs
		}
	}
	if v := os.Getenv("GRUPO_ADMIN_DOMAIN"); v != "" {
		cfg.AdminDomain = v
	}
	if v := os.Getenv("GRUPO_SESSION_SECRET"); v != "" {
		cfg.SessionSecret = v
	}
	if v := os.Getenv("GITHUB_APP_ID"); v != "" {
		cfg.GitHub.AppID = v
	}
	if v := os.Getenv("GITHUB_CLIENT_ID"); v != "" {
		cfg.GitHub.ClientID = v
	}
	if v := os.Getenv("GITHUB_CLIENT_SECRET"); v != "" {
		cfg.GitHub.ClientSecret = v
	}
	if v := os.Getenv("GITHUB_WEBHOOK_SECRET"); v != "" {
		cfg.GitHub.WebhookSecret = v
	}
	if v := os.Getenv("GITHUB_PRIVATE_KEY_PATH"); v != "" {
		cfg.GitHub.PrivateKeyPath = v
	}
	cfg.Dev = envBool("GRUPO_DEV")
	cfg.SkipGitHubAuth = envBool("GRUPO_SKIP_GITHUB_AUTH")
	if v := os.Getenv("GRUPO_VITE_DEV_URL"); v != "" {
		cfg.ViteDevURL = v
	} else if cfg.Dev {
		cfg.ViteDevURL = "http://127.0.0.1:5173"
	}
}

func envBool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes"
}

func (c *Config) Validate() error {
	if c.ListenAddr == "" {
		return fmt.Errorf("listen_addr is required")
	}
	if c.DataDir == "" {
		return fmt.Errorf("data_dir is required")
	}
	if c.AdminDomain == "" {
		return fmt.Errorf("admin_domain is required")
	}
	if !ValidateDomain(c.AdminDomain) {
		return fmt.Errorf("invalid admin_domain: %q", c.AdminDomain)
	}
	if c.SessionSecret == "" {
		return fmt.Errorf("session_secret is required")
	}
	if !c.SkipGitHubAuth {
		if c.GitHub.AppID == "" || c.GitHub.ClientID == "" || c.GitHub.ClientSecret == "" {
			return fmt.Errorf("github app_id, client_id, and client_secret are required")
		}
		if c.GitHub.WebhookSecret == "" {
			return fmt.Errorf("github webhook_secret is required")
		}
		if c.GitHub.PrivateKeyPath == "" {
			return fmt.Errorf("github private_key_path is required")
		}
	}
	return nil
}

func ValidateDomain(domain string) bool {
	if len(domain) == 0 || len(domain) > 253 {
		return false
	}
	return domainPattern.MatchString(domain)
}
