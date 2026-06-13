package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Installation struct {
	ID           int64     `json:"id"`
	AccountLogin string    `json:"account_login"`
	AccountType  string    `json:"account_type"`
	CreatedAt    time.Time `json:"created_at"`
}

type Project struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	InstallationID int64     `json:"installation_id"`
	GitHubOwner    string    `json:"github_owner"`
	GitHubRepo     string    `json:"github_repo"`
	Branch         string    `json:"branch"`
	BuildImage     string    `json:"build_image"`
	BuildCommand   string    `json:"build_command"`
	OutputDir      string    `json:"output_dir"`
	RootDir        string    `json:"root_dir"`
	Domain         string    `json:"domain"`
	SPAFallback    bool      `json:"spa_fallback"`
	EnvJSON        string    `json:"env_json"`
	WebhookID      int64     `json:"webhook_id"`
	FixturePath    string    `json:"fixture_path,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type DeploymentStatus string

const (
	DeploymentQueued  DeploymentStatus = "queued"
	DeploymentRunning DeploymentStatus = "running"
	DeploymentSuccess DeploymentStatus = "success"
	DeploymentFailed  DeploymentStatus = "failed"
)

type Deployment struct {
	ID            string           `json:"id"`
	ProjectID     string           `json:"project_id"`
	Status        DeploymentStatus `json:"status"`
	CommitSHA     string           `json:"commit_sha"`
	CommitMessage string           `json:"commit_message"`
	StartedAt     *time.Time       `json:"started_at,omitempty"`
	FinishedAt    *time.Time       `json:"finished_at,omitempty"`
	LogPath       string           `json:"log_path"`
}

type Session struct {
	ID           string
	GitHubUserID int64
	GitHubLogin  string
	ExpiresAt    time.Time
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	path := filepath.Join(dataDir, "db.sqlite")
	sqlDB, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	store := &Store{db: sqlDB}
	if err := store.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS installations (
	id INTEGER PRIMARY KEY,
	account_login TEXT NOT NULL,
	account_type TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	installation_id INTEGER NOT NULL DEFAULT 0,
	github_owner TEXT NOT NULL DEFAULT '',
	github_repo TEXT NOT NULL DEFAULT '',
	branch TEXT NOT NULL DEFAULT 'main',
	build_image TEXT NOT NULL,
	build_command TEXT NOT NULL,
	output_dir TEXT NOT NULL DEFAULT 'dist',
	root_dir TEXT NOT NULL DEFAULT '',
	domain TEXT NOT NULL UNIQUE,
	spa_fallback INTEGER NOT NULL DEFAULT 0,
	env_json TEXT NOT NULL DEFAULT '{}',
	webhook_id INTEGER NOT NULL DEFAULT 0,
	fixture_path TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS deployments (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	status TEXT NOT NULL,
	commit_sha TEXT NOT NULL DEFAULT '',
	commit_message TEXT NOT NULL DEFAULT '',
	started_at TEXT,
	finished_at TEXT,
	log_path TEXT NOT NULL DEFAULT '',
	FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	github_user_id INTEGER NOT NULL,
	github_login TEXT NOT NULL,
	expires_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_deployments_project ON deployments(project_id);
CREATE INDEX IF NOT EXISTS idx_projects_domain ON projects(domain);
`
	_, err := s.db.Exec(schema)
	return err
}

func (s *Store) UpsertInstallation(ctx context.Context, inst Installation) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO installations (id, account_login, account_type, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET account_login=excluded.account_login, account_type=excluded.account_type
`, inst.ID, inst.AccountLogin, inst.AccountType, inst.CreatedAt.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) DeleteInstallation(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM installations WHERE id = ?`, id)
	return err
}

func (s *Store) ListInstallations(ctx context.Context) ([]Installation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_login, account_type, created_at FROM installations ORDER BY account_login`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Installation
	for rows.Next() {
		var inst Installation
		var created string
		if err := rows.Scan(&inst.ID, &inst.AccountLogin, &inst.AccountType, &created); err != nil {
			return nil, err
		}
		inst.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, inst)
	}
	return out, rows.Err()
}

func (s *Store) CreateProject(ctx context.Context, p *Project) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO projects (
	id, name, installation_id, github_owner, github_repo, branch,
	build_image, build_command, output_dir, root_dir, domain,
	spa_fallback, env_json, webhook_id, fixture_path, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, p.ID, p.Name, p.InstallationID, p.GitHubOwner, p.GitHubRepo, p.Branch,
		p.BuildImage, p.BuildCommand, p.OutputDir, p.RootDir, p.Domain,
		boolToInt(p.SPAFallback), p.EnvJSON, p.WebhookID, p.FixturePath,
		p.CreatedAt.Format(time.RFC3339), p.UpdatedAt.Format(time.RFC3339))
	return err
}

func (s *Store) UpdateProject(ctx context.Context, p Project) error {
	p.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
UPDATE projects SET
	name=?, installation_id=?, github_owner=?, github_repo=?, branch=?,
	build_image=?, build_command=?, output_dir=?, root_dir=?, domain=?,
	spa_fallback=?, env_json=?, webhook_id=?, fixture_path=?, updated_at=?
WHERE id=?
`, p.Name, p.InstallationID, p.GitHubOwner, p.GitHubRepo, p.Branch,
		p.BuildImage, p.BuildCommand, p.OutputDir, p.RootDir, p.Domain,
		boolToInt(p.SPAFallback), p.EnvJSON, p.WebhookID, p.FixturePath,
		p.UpdatedAt.Format(time.RFC3339), p.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) GetProject(ctx context.Context, id string) (*Project, error) {
	row := s.db.QueryRowContext(ctx, projectSelect+" WHERE id = ?", id)
	return scanProject(row)
}

func (s *Store) GetProjectByDomain(ctx context.Context, domain string) (*Project, error) {
	row := s.db.QueryRowContext(ctx, projectSelect+" WHERE domain = ?", domain)
	return scanProject(row)
}

func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, projectSelect+" ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

const projectSelect = `
SELECT id, name, installation_id, github_owner, github_repo, branch,
	build_image, build_command, output_dir, root_dir, domain,
	spa_fallback, env_json, webhook_id, fixture_path, created_at, updated_at
FROM projects
`

func scanProject(scanner interface{ Scan(...any) error }) (*Project, error) {
	var p Project
	var spa int
	var created, updated string
	err := scanner.Scan(
		&p.ID, &p.Name, &p.InstallationID, &p.GitHubOwner, &p.GitHubRepo, &p.Branch,
		&p.BuildImage, &p.BuildCommand, &p.OutputDir, &p.RootDir, &p.Domain,
		&spa, &p.EnvJSON, &p.WebhookID, &p.FixturePath, &created, &updated,
	)
	if err != nil {
		return nil, err
	}
	p.SPAFallback = spa == 1
	p.CreatedAt, _ = time.Parse(time.RFC3339, created)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &p, nil
}

func (s *Store) CreateDeployment(ctx context.Context, d *Deployment) error {
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO deployments (id, project_id, status, commit_sha, commit_message, started_at, finished_at, log_path)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, d.ID, d.ProjectID, d.Status, d.CommitSHA, d.CommitMessage,
		formatTimePtr(d.StartedAt), formatTimePtr(d.FinishedAt), d.LogPath)
	return err
}

func (s *Store) UpdateDeployment(ctx context.Context, d Deployment) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE deployments SET status=?, commit_sha=?, commit_message=?, started_at=?, finished_at=?, log_path=?
WHERE id=?
`, d.Status, d.CommitSHA, d.CommitMessage,
		formatTimePtr(d.StartedAt), formatTimePtr(d.FinishedAt), d.LogPath, d.ID)
	return err
}

func (s *Store) GetDeployment(ctx context.Context, id string) (*Deployment, error) {
	row := s.db.QueryRowContext(ctx, deploymentSelect+" WHERE id = ?", id)
	return scanDeployment(row)
}

func (s *Store) ListDeploymentsByProject(ctx context.Context, projectID string) ([]Deployment, error) {
	rows, err := s.db.QueryContext(ctx, deploymentSelect+" WHERE project_id = ? ORDER BY started_at DESC, id DESC", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Deployment
	for rows.Next() {
		d, err := scanDeployment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

func (s *Store) LatestDeployment(ctx context.Context, projectID string) (*Deployment, error) {
	row := s.db.QueryRowContext(ctx, deploymentSelect+`
WHERE project_id = ?
ORDER BY
	CASE WHEN finished_at IS NOT NULL THEN finished_at ELSE started_at END DESC,
	id DESC
LIMIT 1`, projectID)
	return scanDeployment(row)
}

const deploymentSelect = `
SELECT id, project_id, status, commit_sha, commit_message, started_at, finished_at, log_path
FROM deployments
`

func scanDeployment(scanner interface{ Scan(...any) error }) (*Deployment, error) {
	var d Deployment
	var started, finished sql.NullString
	err := scanner.Scan(&d.ID, &d.ProjectID, &d.Status, &d.CommitSHA, &d.CommitMessage, &started, &finished, &d.LogPath)
	if err != nil {
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

func (s *Store) CreateSession(ctx context.Context, sess Session) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO sessions (id, github_user_id, github_login, expires_at)
VALUES (?, ?, ?, ?)
`, sess.ID, sess.GitHubUserID, sess.GitHubLogin, sess.ExpiresAt.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, github_user_id, github_login, expires_at FROM sessions WHERE id = ?`, id)
	var sess Session
	var expires string
	if err := row.Scan(&sess.ID, &sess.GitHubUserID, &sess.GitHubLogin, &expires); err != nil {
		return nil, err
	}
	sess.ExpiresAt, _ = time.Parse(time.RFC3339, expires)
	return &sess, nil
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, time.Now().UTC().Format(time.RFC3339))
	return err
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func formatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}
