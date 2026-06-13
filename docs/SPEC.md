# Grupo — Product Spec (v1)

A self-hostable static site deployment service inspired by Cloudflare Pages. Install on a Linux machine (e.g. Mac Mini), connect GitHub, configure domains, and serve multiple sites behind a Cloudflare Tunnel.

**Design goal for v1:** one systemd-managed binary, minimal moving parts, no Kubernetes. Builds run in Docker containers; the Grupo service itself runs natively on the host.

---

## Decisions (locked in)

| Decision | Choice |
|---|---|
| **Name** | `grupo` |
| **Builds** | Each build runs in a Docker container (configurable image per project) |
| **GitHub** | GitHub App — sign-in triggers installation; webhooks auto-configured on project create |
| **Monorepos** | `root_dir` field on project (optional subpath within repo) |
| **UI** | React + Tailwind (Vite), embedded in the Go binary at compile time |
| **Install** | systemd binary (`/usr/local/bin/grupo`) — Docker required on host for builds |

---

## Problem

Cloudflare Pages is convenient but couples hosting to Cloudflare. For a home lab or small fleet of static sites, you want:

- Git push → build → deploy, without a SaaS dependency
- Multiple sites on one machine, routed by hostname
- A single ingress point compatible with `cloudflared` tunnel routing

---

## Non-Goals (v1)

| Out of scope | Reason |
|---|---|
| Preview deployments per PR | Adds branch routing complexity |
| Build caching / incremental builds | Optimize later |
| Custom SSL termination | Cloudflare Tunnel handles TLS |
| Multi-user RBAC | Single admin is enough for home lab |
| Non-GitHub Git hosts | GitHub only in v1 |
| Server-side rendering / SSR | Static files only |
| Docker Compose install for Grupo itself | systemd binary only; Docker used for builds |

---

## Target User Flow

1. Install Docker and the Grupo binary on Linux (`install.sh` → systemd unit).
2. Point a Cloudflare Tunnel at the service HTTP port (e.g. `:8080`).
3. Open the admin UI, sign in with GitHub (install the Grupo GitHub App if prompted).
4. Create a **project**: pick repo, branch, build image, build command, output directory, root dir (optional), and hostname.
5. Grupo automatically registers a webhook on the repo via the GitHub App installation — no manual GitHub settings.
6. Push to the configured branch → Docker build runs → new files go live at the hostname.
7. Repeat for additional projects; each gets its own hostname on the same tunnel.

---

## Architecture

```
                    ┌─────────────────────────────────────────┐
                    │           Cloudflare Tunnel             │
                    │   *.yourdomain.com → localhost:8080     │
                    └────────────────────┬────────────────────┘
                                         │
                    ┌────────────────────▼────────────────────┐
                    │              HTTP Router                  │
                    │  Host: admin.yourdomain.com → React UI    │
                    │  Host: app1.yourdomain.com  → Site A      │
                    │  Host: app2.yourdomain.com  → Site B      │
                    └─────────┬───────────────────┬───────────┘
                              │                   │
                    ┌─────────▼─────────┐ ┌───────▼──────────┐
                    │   Control Plane   │ │  Static File     │
                    │   (Go API + UI)   │ │  Server          │
                    └─────────┬─────────┘ └───────▲──────────┘
                              │                   │
                    ┌─────────▼─────────┐         │
                    │   Build Worker    │─────────┘
                    │   (queue + jobs)  │  writes to deploy dirs
                    └─────────┬─────────┘
                              │ docker run
                    ┌─────────▼─────────┐
                    │   Build Container │  (ephemeral, per job)
                    └─────────┬─────────┘
                              │
                    ┌─────────▼─────────┐
                    │   SQLite + FS     │
                    │   /var/lib/grupo/ │
                    └───────────────────┘
```

### Components (v1)

| Component | Responsibility |
|---|---|
| **Router** | Single HTTP listener; routes by `Host` header to embedded React UI or static site roots |
| **Control plane** | REST API, GitHub App auth, project CRUD, webhook ingestion |
| **Build worker** | Clone repo, run build in Docker container, atomically swap deployment directory |
| **Storage** | SQLite for metadata; filesystem for clones, logs, and published assets |

### Stack

| Layer | Choice |
|---|---|
| **Backend** | Go — single static binary, embeds frontend assets |
| **Database** | SQLite (`modernc.org/sqlite`) |
| **Frontend** | React + Vite + Tailwind CSS |
| **Auth** | GitHub App (user-to-server OAuth + installation tokens) |
| **Builds** | Docker Engine on host (`/var/run/docker.sock`) |

**Repo layout (planned):**

```
grupo/
├── cmd/grupo/          # main entrypoint
├── internal/           # api, router, build, github, db, config
├── web/                # React + Tailwind (Vite)
├── scripts/install.sh
└── docs/SPEC.md
```

---

## Data Model

### `installations`

Tracks GitHub App installations linked to the instance.

| Column | Type | Notes |
|---|---|---|
| `id` | int | GitHub installation ID (PK) |
| `account_login` | string | User or org login |
| `account_type` | string | `User` or `Organization` |
| `created_at` | timestamp | |

### `projects`

| Column | Type | Notes |
|---|---|---|
| `id` | UUID | Primary key |
| `name` | string | Display name |
| `installation_id` | int | FK → `installations.id` |
| `github_owner` | string | e.g. `octocat` |
| `github_repo` | string | e.g. `my-site` |
| `branch` | string | Trigger branch, e.g. `main` |
| `build_image` | string | Docker image, e.g. `node:22-bookworm` |
| `build_command` | string | e.g. `npm ci && npm run build` |
| `output_dir` | string | Relative path inside repo, e.g. `dist` |
| `root_dir` | string | Optional monorepo subpath, e.g. `apps/web` |
| `domain` | string | Unique hostname, e.g. `app1.example.com` |
| `spa_fallback` | bool | Unknown paths → `index.html` |
| `env_json` | JSON | Build-time env vars (optional v1) |
| `webhook_id` | int | GitHub hook ID (for cleanup on delete) |
| `created_at` | timestamp | |
| `updated_at` | timestamp | |

### `deployments`

| Column | Type | Notes |
|---|---|---|
| `id` | UUID | |
| `project_id` | UUID | FK |
| `status` | enum | `queued`, `running`, `success`, `failed` |
| `commit_sha` | string | |
| `commit_message` | string | |
| `started_at` | timestamp | |
| `finished_at` | timestamp | |
| `log_path` | string | Path to build log file |

### `sessions`

| Column | Type | Notes |
|---|---|---|
| `id` | string | Session token |
| `github_user_id` | int | |
| `github_login` | string | |
| `expires_at` | timestamp | |

---

## Filesystem Layout

```
/var/lib/grupo/
├── db.sqlite
├── repos/                    # ephemeral clone workspaces
│   └── {project_id}/
├── sites/                    # published static assets
│   └── {project_id}/
│       ├── current/          # symlink → releases/{deployment_id}
│       └── releases/
│           └── {deployment_id}/
└── logs/
    └── {deployment_id}.log
```

**Atomic deploy:** build into `releases/{deployment_id}/`, then `ln -sfn` swap `current` symlink. Rollback in v2 = repoint symlink.

---

## HTTP Routing

Single port (default `8080`). Route table loaded from SQLite on change (in-memory cache + reload on project update).

| Host | Handler |
|---|---|
| `ADMIN_DOMAIN` (config) | Embedded React SPA + `/api/*` |
| `{project.domain}` | Serve files from `/var/lib/grupo/sites/{project_id}/current` |
| Unknown host | 404 plain text |

**Static serving behavior:**

- Try exact file path
- Fall back to `index.html` for directories
- SPA fallback (per-project `spa_fallback` flag): unknown paths → `index.html`

**Frontend routing:** React Router handles client-side routes under the admin domain. API lives at `/api/*`. Webhook at `/webhooks/github`.

---

## GitHub App Integration

Grupo uses a **GitHub App** (not a standalone OAuth App). One app registration per Grupo instance (or shared across instances — document both options).

### App permissions (minimum)

| Permission | Access | Why |
|---|---|---|
| Repository metadata | Read | Resolve repo info |
| Contents | Read | Clone via HTTPS with installation token |
| Webhooks | Read & write | Auto-create repo hooks on project create |
| Administration | Read | List repos accessible to installation |

### Subscribe to events

- `push` — trigger builds
- `installation` / `installation_repositories` — track install/uninstall

### Sign-in flow

```
User clicks "Sign in with GitHub"
        │
        ▼
Redirect to GitHub App install/OAuth authorize
        │
        ├── Not installed → user selects account/orgs → Install
        └── Already installed → authorize user
        │
        ▼
Callback: exchange code for user token + store installation ID(s)
        │
        ▼
Session cookie set → React dashboard
```

- User-to-server token for listing installations and repos the user can access.
- Installation token (short-lived, refreshed per API call) for repo operations and webhook management.

### Automatic webhooks

When a project is created:

1. Grupo calls `POST /repos/{owner}/{repo}/hooks` with the installation token.
2. Payload URL: `https://{ADMIN_DOMAIN}/webhooks/github`
3. Secret: app-level `GITHUB_WEBHOOK_SECRET` (same for all hooks).
4. Events: `push`
5. Store returned `webhook_id` on the project for cleanup on delete.

When a project is deleted:

- `DELETE /repos/{owner}/{repo}/hooks/{webhook_id}`

### Webhook handler

Single endpoint: `POST /webhooks/github`

- Verify `X-Hub-Signature-256` with app webhook secret.
- On `push`: match payload repo + branch against projects → enqueue build.
- On `installation` events: upsert/remove rows in `installations`.

---

## Build Pipeline (Docker)

```
Webhook / manual trigger
        │
        ▼
  Enqueue deployment (status: queued)
        │
        ▼
  Worker picks job (single concurrent build in v1)
        │
        ├── git clone --depth 1 --branch {branch}
        │   (using installation token in clone URL)
        ├── prepare workspace under /var/lib/grupo/repos/{project_id}
        │
        ├── docker run --rm
        │     -v {workspace}:/src
        │     -v {staging_out}:/out
        │     -w /src/{root_dir}
        │     -e ... (env_json)
        │     {build_image}
        │     bash -lc "{build_command} && cp -r {output_dir}/. /out/"
        │
        ├── verify /out is non-empty
        ├── copy /out → sites/{project_id}/releases/{deployment_id}/
        ├── atomic symlink swap on current
        └── status: success | failed + log (stdout/stderr captured)
```

**Default build images (UI presets):**

| Preset | Image |
|---|---|
| Node.js 22 | `node:22-bookworm` |
| Node.js 20 | `node:20-bookworm` |
| Static (no build) | `alpine:3.20` |

User can enter any public image (e.g. `golang:1.22`, `oven/bun:1`).

**Docker requirements:**

- Grupo systemd unit needs access to `/var/run/docker.sock`
- Document minimum Docker version in README
- Build containers run as `--rm` with no privileged flag

**Concurrency:** One build at a time globally in v1 (queue additional jobs). Avoids CPU/RAM contention on a Mac Mini.

**Manual redeploy:** `POST /api/projects/{id}/deploy` — rebuild latest commit on branch.

---

## API (v1)

Session auth required on `/api/*`. Webhook uses GitHub signature auth.

| Method | Path | Description |
|---|---|---|
| GET | `/auth/github` | Start GitHub App user OAuth |
| GET | `/auth/github/callback` | OAuth callback |
| POST | `/auth/logout` | End session |
| GET | `/api/me` | Current user + installations |
| GET | `/api/github/repos` | List repos for installation (`?installation_id=`) |
| GET | `/api/projects` | List projects |
| POST | `/api/projects` | Create project (+ auto webhook) |
| GET | `/api/projects/{id}` | Get project |
| PATCH | `/api/projects/{id}` | Update project |
| DELETE | `/api/projects/{id}` | Delete project, webhook, and files |
| POST | `/api/projects/{id}/deploy` | Trigger manual build |
| GET | `/api/projects/{id}/deployments` | List deployments |
| GET | `/api/deployments/{id}` | Deployment detail |
| GET | `/api/deployments/{id}/logs` | Return build log (text) |
| POST | `/webhooks/github` | GitHub App webhook receiver |
| GET | `/healthz` | Health check |

---

## Admin UI (React + Tailwind)

Vite dev server proxies to Go API during development. Production build embedded via `go:embed`.

### Screens

1. **Login** — “Sign in with GitHub” (redirects to App install/authorize)
2. **Dashboard** — project cards: name, domain, branch, last deploy status, link to live site
3. **New project**
   - Select installation (if user has multiple org installs)
   - Select repo (searchable dropdown)
   - Branch (default `main`)
   - Build image (preset dropdown + custom input)
   - Build command
   - Output directory (default `dist`)
   - Root directory (optional, for monorepos)
   - Domain hostname
   - SPA fallback toggle
4. **Project detail**
   - Edit settings
   - Deployment history table
   - Log viewer (monospace, scrollable)
   - “Deploy now” button
5. **Settings** — read-only instance info (admin domain, data dir, GitHub App status)

Keep v1 UI functional and clean with Tailwind — no component library required, but shadcn/ui is an optional upgrade later.

---

## Configuration

`/etc/grupo/config.yaml` (or environment variables):

```yaml
listen_addr: ":8080"
data_dir: "/var/lib/grupo"
admin_domain: "grupo-admin.example.com"
github:
  app_id: "123456"
  client_id: "Iv1.xxxx"           # GitHub App client ID (OAuth)
  client_secret: "..."            # GitHub App client secret
  webhook_secret: "..."           # App webhook secret
  private_key_path: "/etc/grupo/github-app.pem"
session_secret: "random-32-bytes"
```

Environment variable overrides (for systemd `EnvironmentFile`):

```
GRUPO_LISTEN_ADDR=:8080
GRUPO_DATA_DIR=/var/lib/grupo
GRUPO_ADMIN_DOMAIN=grupo-admin.example.com
GITHUB_APP_ID=123456
GITHUB_CLIENT_ID=Iv1.xxxx
GITHUB_CLIENT_SECRET=...
GITHUB_WEBHOOK_SECRET=...
GITHUB_PRIVATE_KEY_PATH=/etc/grupo/github-app.pem
GRUPO_SESSION_SECRET=...
```

### GitHub App setup (one-time, documented in README)

1. Create GitHub App at `github.com/settings/apps/new`
2. Set callback URL: `https://{ADMIN_DOMAIN}/auth/github/callback`
3. Set webhook URL: `https://{ADMIN_DOMAIN}/webhooks/github`
4. Generate private key, download PEM to `/etc/grupo/github-app.pem`
5. Paste app ID, client ID/secret, webhook secret into config

---

## Cloudflare Tunnel Setup

Document in README; not built into the app.

**Recommended — wildcard ingress:**

```yaml
tunnel: <tunnel-id>
credentials-file: /path/to/credentials.json

ingress:
  - hostname: "*.example.com"
    service: http://localhost:8080
  - service: http_status:404
```

Grupo routes by `Host` internally — no per-site tunnel rules needed.

---

## Security Considerations (v1)

- Validate GitHub webhook signatures (app secret)
- Session cookies: HttpOnly, Secure, SameSite=Lax
- Build commands run inside Docker containers — still arbitrary code; use trusted images and trusted admin only
- Mount only the clone workspace and output dir into build containers — no Docker socket inside containers
- Sanitize `domain` (alphanumeric, dots, hyphens only)
- Rate-limit webhook endpoint
- Do not expose `/var/lib/grupo` via HTTP
- Installation tokens are short-lived and never sent to the frontend

---

## Installation (systemd binary)

**Prerequisites:** Linux, Docker Engine, systemd

```bash
curl -fsSL https://.../install.sh | sudo bash
```

The install script:

1. Downloads `grupo` binary to `/usr/local/bin/grupo`
2. Creates `/var/lib/grupo` and `/etc/grupo/`
3. Installs systemd unit `grupo.service`
4. Adds `grupo` user to `docker` group (or documents socket permissions)
5. Enables and starts the service

**systemd unit (sketch):**

```ini
[Unit]
Description=Grupo static site host
After=network-online.target docker.service
Requires=docker.service

[Service]
Type=simple
User=grupo
Group=grupo
EnvironmentFile=/etc/grupo/env
ExecStart=/usr/local/bin/grupo serve
Restart=on-failure
ReadWritePaths=/var/lib/grupo

[Install]
WantedBy=multi-user.target
```

**Build from source:**

```bash
make build   # builds web/ → embed → go build
sudo make install
```

---

## Implementation Plan

### Milestone 0 — Scaffold

- [ ] Repo layout: `cmd/`, `internal/`, `web/`
- [ ] Go module, config loading, SQLite migrations
- [ ] HTTP server skeleton: `GET /healthz`
- [ ] Vite + React + Tailwind scaffold in `web/`
- [ ] Makefile: `make dev`, `make build` (embed frontend)
- [ ] CI: lint, test, build

### Milestone 1 — Routing + Static Serve

- [ ] Host-based router
- [ ] Project CRUD API (no auth yet)
- [ ] Serve static files from `sites/{id}/current`
- [ ] SPA fallback flag
- [ ] Manual fixture deploy to verify routing

### Milestone 2 — GitHub App Auth + React UI

- [ ] GitHub App OAuth flow + sessions
- [ ] Track installations in DB
- [ ] List repos via installation token
- [ ] React: login, dashboard, new project form
- [ ] Protect `/api/*` routes

### Milestone 3 — Docker Build Worker

- [ ] Deployment queue (in-process, single worker)
- [ ] Git clone with installation token
- [ ] Docker build runner (mount workspace, capture logs)
- [ ] Publish to `releases/`, atomic symlink swap
- [ ] Build log API + log viewer in React

### Milestone 4 — Webhooks

- [ ] App webhook endpoint + signature verification
- [ ] Auto-create repo webhook on project create
- [ ] Auto-delete webhook on project delete
- [ ] Branch filter on push events
- [ ] Handle installation lifecycle events

### Milestone 5 — Ship

- [ ] `install.sh` + systemd unit
- [ ] README: GitHub App creation, tunnel setup, first deploy walkthrough
- [ ] Error states in UI (failed builds, missing installation)
- [ ] Basic integration test with fixture repo + `alpine` build image

---

## Testing Strategy

| Layer | Approach |
|---|---|
| Unit | Router matching, webhook signature verification, config validation |
| Integration | Docker build with fixture repo (`alpine` + echo to `dist/`) |
| Frontend | Vitest + React Testing Library for critical forms (optional v1) |
| E2E | curl with `Host` header; simulated signed webhook POST |

Fixture project for CI:

```yaml
build_image: alpine:3.20
build_command: mkdir -p dist && echo hello > dist/index.html
output_dir: dist
```

---

## Open Questions

1. **Custom 404 pages** — per-project `404.html` support: defer to v2?
2. **Shared vs per-instance GitHub App** — document creating your own app vs a published Grupo app?
3. **Build concurrency** — stay at 1 global worker for v1, or allow N parallel?

---

## Success Criteria (v1 Done)

- [ ] Install on Linux via `install.sh` (systemd + Docker)
- [ ] Sign in with GitHub; App installs without manual webhook setup
- [ ] Create two projects with different domains and build images
- [ ] Push to branch → Docker build → site updates
- [ ] Cloudflare Tunnel wildcard routes both sites correctly
- [ ] Build logs visible in React UI
- [ ] Failed build does not take down previous successful deployment
- [ ] Deleting a project removes its GitHub webhook

---

## Future (v2+)

- Preview URLs per branch (`branch---project.example.com`)
- Rollback to previous deployment
- Environment variables UI + secrets encryption at rest
- Build concurrency per project
- Custom 404 / `_redirects` file support
- Metrics (Prometheus)
- Email/Slack notifications on deploy failure
- shadcn/ui polish pass
