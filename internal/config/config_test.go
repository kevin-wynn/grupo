package config

import (
	"os"
	"testing"
)

func TestValidateDomain(t *testing.T) {
	tests := []struct {
		domain string
		ok     bool
	}{
		{"admin.mygrupo.dev", true},
		{"app1.mygrupo.dev", true},
		{"invalid_domain!", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := ValidateDomain(tt.domain); got != tt.ok {
			t.Errorf("ValidateDomain(%q) = %v, want %v", tt.domain, got, tt.ok)
		}
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("GRUPO_LISTEN_ADDR", ":9090")
	t.Setenv("GRUPO_DATA_DIR", "/tmp/grupo-test")
	t.Setenv("GRUPO_ADMIN_DOMAIN", "admin.example.com")
	t.Setenv("GRUPO_SESSION_SECRET", "test-secret")
	t.Setenv("GRUPO_SKIP_GITHUB_AUTH", "true")

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != ":9090" {
		t.Fatalf("listen_addr = %q", cfg.ListenAddr)
	}
	if !cfg.SkipGitHubAuth {
		t.Fatal("expected skip github auth")
	}
}

func TestLoadRequiresSessionSecret(t *testing.T) {
	os.Unsetenv("GRUPO_SESSION_SECRET")
	t.Setenv("GRUPO_SKIP_GITHUB_AUTH", "true")
	_, err := Load("")
	if err == nil {
		t.Fatal("expected error for missing session secret")
	}
}
