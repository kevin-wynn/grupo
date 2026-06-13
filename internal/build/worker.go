package build

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kevin-wynn/grupo/internal/db"
	"github.com/kevin-wynn/grupo/internal/github"
)

type TokenProvider func(ctx context.Context, installationID int64) (string, error)

type Worker struct {
	store         *db.Store
	dataDir       string
	tokenProvider TokenProvider
	devMode       bool
	mu            sync.Mutex
	running       bool
}

func NewWorker(store *db.Store, dataDir string, tokenProvider TokenProvider, devMode bool) *Worker {
	return &Worker{
		store:         store,
		dataDir:       dataDir,
		tokenProvider: tokenProvider,
		devMode:       devMode,
	}
}

type Job struct {
	ProjectID     string
	CommitSHA     string
	CommitMessage string
}

func (w *Worker) EnqueueSync(ctx context.Context, job Job) (*db.Deployment, error) {
	d, err := w.createDeployment(ctx, job)
	if err != nil {
		return nil, err
	}
	w.runDeployment(ctx, d)
	updated, err := w.store.GetDeployment(ctx, d.ID)
	if err != nil {
		return d, nil
	}
	return updated, nil
}

func (w *Worker) Enqueue(ctx context.Context, job Job) (*db.Deployment, error) {
	d, err := w.createDeployment(ctx, job)
	if err != nil {
		return nil, err
	}
	go w.processNext()
	return d, nil
}

func (w *Worker) createDeployment(ctx context.Context, job Job) (*db.Deployment, error) {
	if job.ProjectID == "" {
		return nil, fmt.Errorf("project_id required")
	}
	d := db.Deployment{
		ProjectID:     job.ProjectID,
		Status:        db.DeploymentQueued,
		CommitSHA:     job.CommitSHA,
		CommitMessage: job.CommitMessage,
	}
	if err := w.store.CreateDeployment(ctx, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (w *Worker) processNext() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
	}()

	ctx := context.Background()
	for {
		d, err := w.nextQueued(ctx)
		if err != nil || d == nil {
			return
		}
		w.runDeployment(ctx, d)
	}
}

func (w *Worker) nextQueued(ctx context.Context) (*db.Deployment, error) {
	row := w.store.DB().QueryRowContext(ctx, `
SELECT id, project_id, status, commit_sha, commit_message, started_at, finished_at, log_path
FROM deployments WHERE status = ? ORDER BY rowid LIMIT 1`, db.DeploymentQueued)
	var d db.Deployment
	var started, finished sql.NullString
	if err := row.Scan(&d.ID, &d.ProjectID, &d.Status, &d.CommitSHA, &d.CommitMessage, &started, &finished, &d.LogPath); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if started.Valid {
		t, _ := time.Parse(time.RFC3339, started.String)
		d.StartedAt = &t
	}
	if finished.Valid {
		t, _ := time.Parse(time.RFC3339, finished.String)
		d.FinishedAt = &t
	}
	return &d, nil
}

func (w *Worker) runDeployment(ctx context.Context, d *db.Deployment) {
	now := time.Now().UTC()
	d.Status = db.DeploymentRunning
	d.StartedAt = &now

	logPath := filepath.Join(w.dataDir, "logs", d.ID+".log")
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	d.LogPath = logPath
	_ = w.store.UpdateDeployment(ctx, *d)

	logFile, err := os.Create(logPath)
	if err != nil {
		w.fail(ctx, d, fmt.Sprintf("create log: %v", err))
		return
	}
	defer logFile.Close()
	logger := io.MultiWriter(logFile, os.Stdout)

	project, err := w.store.GetProject(ctx, d.ProjectID)
	if err != nil {
		w.failWithLog(ctx, d, logger, "load project: %v", err)
		return
	}

	if err := w.executeBuild(ctx, project, d, logger); err != nil {
		w.failWithLog(ctx, d, logger, "build failed: %v", err)
		return
	}

	finished := time.Now().UTC()
	d.Status = db.DeploymentSuccess
	d.FinishedAt = &finished
	_ = w.store.UpdateDeployment(ctx, *d)
	fmt.Fprintf(logger, "deployment succeeded\n")
}

func (w *Worker) executeBuild(ctx context.Context, project *db.Project, d *db.Deployment, logger io.Writer) error {
	repoDir := filepath.Join(w.dataDir, "repos", project.ID)
	_ = os.RemoveAll(repoDir)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		return err
	}

	if project.FixturePath != "" {
		fmt.Fprintf(logger, "using fixture path %s\n", project.FixturePath)
		if err := copyDir(project.FixturePath, repoDir); err != nil {
			return fmt.Errorf("copy fixture: %w", err)
		}
	} else {
		token := ""
		if w.tokenProvider != nil && project.InstallationID > 0 {
			var err error
			token, err = w.tokenProvider(ctx, project.InstallationID)
			if err != nil {
				return fmt.Errorf("installation token: %w", err)
			}
		}
		cloneURL := github.CloneURLWithToken(project.GitHubOwner, project.GitHubRepo, token)
		fmt.Fprintf(logger, "cloning %s/%s branch %s\n", project.GitHubOwner, project.GitHubRepo, project.Branch)
		if err := runCmd(ctx, repoDir, logger, "git", "clone", "--depth", "1", "--branch", project.Branch, cloneURL, "."); err != nil {
			return err
		}
	}

	if project.RootDir != "" {
		_ = filepath.Join(repoDir, project.RootDir)
	}

	stagingOut := filepath.Join(w.dataDir, "tmp", d.ID)
	_ = os.RemoveAll(stagingOut)
	if err := os.MkdirAll(stagingOut, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(stagingOut)

	workInContainer := "/src"
	if project.RootDir != "" {
		workInContainer = "/src/" + strings.Trim(project.RootDir, "/")
	}

	buildScript := fmt.Sprintf("%s && cp -r %s/. /out/", project.BuildCommand, project.OutputDir)

	if project.FixturePath != "" && w.devMode && !dockerAvailable() {
		fmt.Fprintf(logger, "docker unavailable; running local fixture build\n")
		localOut := filepath.Join(w.dataDir, "tmp", d.ID+"-local")
		_ = os.RemoveAll(localOut)
		if err := os.MkdirAll(localOut, 0o755); err != nil {
			return err
		}
		defer os.RemoveAll(localOut)
		workDir := repoDir
		if project.RootDir != "" {
			workDir = filepath.Join(repoDir, project.RootDir)
		}
		localScript := fmt.Sprintf("%s && cp -r %s/. %s/", project.BuildCommand, project.OutputDir, localOut)
		if err := runCmd(ctx, workDir, logger, "sh", "-lc", localScript); err != nil {
			return err
		}
		if err := copyDir(localOut, stagingOut); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(logger, "running docker build with image %s\n", project.BuildImage)
		dockerArgs := []string{
			"run", "--rm",
			"-v", fmt.Sprintf("%s:/src", repoDir),
			"-v", fmt.Sprintf("%s:/out", stagingOut),
			"-w", workInContainer,
			project.BuildImage,
			"sh", "-lc", buildScript,
		}
		if err := runCmd(ctx, "", logger, "docker", dockerArgs...); err != nil {
			return err
		}
	}

	if empty, err := dirEmpty(stagingOut); err != nil {
		return err
	} else if empty {
		return fmt.Errorf("build output is empty")
	}

	releaseDir := filepath.Join(w.dataDir, "sites", project.ID, "releases", d.ID)
	if err := os.RemoveAll(releaseDir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(releaseDir), 0o755); err != nil {
		return err
	}
	if err := copyDir(stagingOut, releaseDir); err != nil {
		return err
	}

	currentLink := filepath.Join(w.dataDir, "sites", project.ID, "current")
	if err := os.MkdirAll(filepath.Dir(currentLink), 0o755); err != nil {
		return err
	}
	tmpLink := currentLink + ".tmp"
	_ = os.Remove(tmpLink)
	if err := os.Symlink(releaseDir, tmpLink); err != nil {
		return err
	}
	if err := os.Rename(tmpLink, currentLink); err != nil {
		return err
	}

	return nil
}

func (w *Worker) fail(ctx context.Context, d *db.Deployment, msg string) {
	finished := time.Now().UTC()
	d.Status = db.DeploymentFailed
	d.FinishedAt = &finished
	_ = w.store.UpdateDeployment(ctx, *d)
	fmt.Println(msg)
}

func (w *Worker) failWithLog(ctx context.Context, d *db.Deployment, logger io.Writer, format string, args ...any) {
	fmt.Fprintf(logger, format+"\n", args...)
	w.fail(ctx, d, fmt.Sprintf(format, args...))
}

func runCmd(ctx context.Context, dir string, logger io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = logger
	cmd.Stderr = logger
	return cmd.Run()
}

func dirEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func dockerAvailable() bool {
	_, err := exec.LookPath("docker")
	if err != nil {
		return false
	}
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

// RunDeploymentSync runs a deployment synchronously (for tests).
func (w *Worker) RunDeploymentSync(ctx context.Context, d *db.Deployment) {
	w.runDeployment(ctx, d)
}
