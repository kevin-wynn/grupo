package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/kevin-wynn/grupo/internal/api"
	"github.com/kevin-wynn/grupo/internal/auth"
	"github.com/kevin-wynn/grupo/internal/build"
	"github.com/kevin-wynn/grupo/internal/config"
	"github.com/kevin-wynn/grupo/internal/db"
	gh "github.com/kevin-wynn/grupo/internal/github"
	"github.com/kevin-wynn/grupo/internal/server"
	"github.com/kevin-wynn/grupo/internal/webembed"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "serve":
		if err := runServe(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "seed":
		if err := runSeed(); err != nil {
			log.Fatal(err)
		}
	case "deploy-fixture":
		if err := runDeployFixture(); err != nil {
			log.Fatal(err)
		}
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("usage: grupo serve [flags]")
	fmt.Println("       grupo seed")
	fmt.Println("       grupo deploy-fixture")
}

func loadConfig() (*config.Config, error) {
	_ = godotenv.Load(".env.local")
	configPath := os.Getenv("GRUPO_CONFIG")
	return config.Load(configPath)
}

func runServe(args []string) error {
	serveFlags := flag.NewFlagSet("serve", flag.ContinueOnError)
	if err := serveFlags.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	for _, dir := range []string{
		cfg.DataDir,
		filepath.Join(cfg.DataDir, "logs"),
		filepath.Join(cfg.DataDir, "repos"),
		filepath.Join(cfg.DataDir, "sites"),
		filepath.Join(cfg.DataDir, "tmp"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	store, err := db.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer store.Close()

	var ghClient *gh.Client
	if !cfg.SkipGitHubAuth {
		ghClient, err = gh.NewClient(
			cfg.GitHub.AppID,
			cfg.GitHub.ClientID,
			cfg.GitHub.ClientSecret,
			cfg.GitHub.PrivateKeyPath,
		)
		if err != nil {
			return err
		}
	}

	tokenProvider := func(ctx context.Context, installationID int64) (string, error) {
		if ghClient == nil {
			return "", fmt.Errorf("github client not configured")
		}
		return ghClient.InstallationToken(ctx, installationID)
	}

	worker := build.NewWorker(store, cfg.DataDir, tokenProvider, cfg.Dev)
	sessions := auth.NewManager(store, !cfg.Dev)
	webFS, _ := fs.Sub(webembed.FS, "dist")

	srv, err := server.New(cfg, store, nil, webFS)
	if err != nil {
		return err
	}
	apiHandler := api.NewHandler(cfg, store, worker, ghClient, sessions, srv.ReloadRoutes)
	srv, err = server.New(cfg, store, apiHandler, webFS)
	if err != nil {
		return err
	}

	log.Printf("grupo listening on %s (admin: %s)", cfg.ListenAddr, cfg.AdminDomain)
	return http.ListenAndServe(cfg.ListenAddr, srv.Handler())
}

func runSeed() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	store, err := db.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer store.Close()

	fixturePath, err := filepath.Abs("fixtures/hello-site")
	if err != nil {
		return err
	}
	p, err := api.SeedFixtureProject(context.Background(), store, fixturePath)
	if err != nil {
		return err
	}
	fmt.Printf("seeded project %s (%s)\n", p.Name, p.ID)
	return nil
}

func runDeployFixture() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	store, err := db.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer store.Close()

	fixturePath, err := filepath.Abs("fixtures/hello-site")
	if err != nil {
		return err
	}
	p, err := api.SeedFixtureProject(context.Background(), store, fixturePath)
	if err != nil {
		return err
	}

	var ghClient *gh.Client
	if !cfg.SkipGitHubAuth {
		ghClient, err = gh.NewClient(
			cfg.GitHub.AppID,
			cfg.GitHub.ClientID,
			cfg.GitHub.ClientSecret,
			cfg.GitHub.PrivateKeyPath,
		)
		if err != nil {
			return err
		}
	}
	tokenProvider := func(ctx context.Context, installationID int64) (string, error) {
		if ghClient == nil {
			return "", fmt.Errorf("github client not configured")
		}
		return ghClient.InstallationToken(ctx, installationID)
	}
	worker := build.NewWorker(store, cfg.DataDir, tokenProvider, cfg.Dev)
	d, err := worker.EnqueueSync(context.Background(), build.Job{
		ProjectID:     p.ID,
		CommitMessage: "fixture deploy",
	})
	if err != nil {
		return err
	}
	fmt.Printf("deployment %s finished with status %s\n", d.ID, d.Status)
	return nil
}
