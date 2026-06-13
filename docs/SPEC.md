# Self-Hosted Pages — Product Spec (v1)

A self-hostable static site deployment service inspired by Cloudflare Pages. Install on a Linux machine (e.g. Mac Mini), connect GitHub, configure domains, and serve multiple sites behind a Cloudflare Tunnel.

**Design goal for v1:** one binary (or one `docker compose up`), minimal moving parts, no Kubernetes.

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
| Container-isolated builds | Optional in v1; subprocess + workdir first |

---

## Target User Flow

1. Install the service on Linux (systemd or Docker Compose).
2. Point a Cloudflare Tunnel at the service HTTP port (e.g. `:8080`).
3. Open the admin UI, sign in with GitHub.
4. Create a **project**: pick repo, branch, build command, output directory, and hostname.
5. Register a GitHub webhook (manual paste URL in v1, or auto if using a GitHub App).
6. Push to the configured branch → build runs → new files go live at the hostname.
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
                    │  Host: admin.yourdomain.com → Admin UI    │
                    │  Host: app1.yourdomain.com  → Site A      │
                    │  Host: app2.yourdomain.com  → Site B      │
                    └─────────┬───────────────────┬───────────┘
                              │                   │
                    ┌─────────▼─────────┐ ┌───────▼──────────┐
                    │   Control Plane   │ │  Static File     │
                    │   (API + UI)      │ │  Server          │
                    └─────────┬─────────┘ └───────▲──────────┘
                              │                   │
                    ┌─────────▼─────────┐         │
                    │   Build Worker    │─────────┘
                    │   (queue + jobs)  │  writes to deploy dirs
                    └─────────┬─────────┘
                              │
                    ┌─────────▼─────────┐
                    │   SQLite + FS     │
                    │   /data/          │
                    └───────────────────┘
```

### Components (v1)

| Component | Responsibility |
|---|---|
| **Router** | Single HTTP listener; routes by `Host` header to admin UI or static site roots |
| **Control plane** | REST API, web UI, GitHub OAuth, project CRUD, webhook ingestion |
| **Build worker** | Clone repo, run build command, atomically swap deployment directory |
| **Storage** | SQLite for metadata; filesystem for builds, logs, and published assets |

**Recommended stack (opinionated for simplicity):**

- **Language:** Go — single static binary, good HTTP server, easy cross-compile
- **Database:** SQLite (`modernc.org/sqlite` or `github.com/mattn/go-sqlite3`)
- **UI:** Server-rendered HTML + minimal HTMX, or a small React/Vite SPA served from the binary
- **Auth:** GitHub OAuth App (not GitHub App in v1 — fewer setup steps)

Alternative: Python (FastAPI) + SQLite if the team prefers Python. The spec is stack-agnostic; Go is suggested for install simplicity.

---

## Data Model

### `projects`

| Column | Type | Notes |
|---|---|---|
| `id` | UUID | Primary key |
| `name` | string | Display name |
| `github_owner` | string | e.g. `octocat` |
| `github_repo` | string | e.g. `my-site` |
| `branch` | string | Trigger branch, e.g. `main` |
| `build_command` | string | e.g. `npm ci && npm run build` |
| `output_dir` | string | Relative path inside repo, e.g. `dist` |
| `root_dir` | string | Optional monorepo subpath |
| `domain` | string | Unique hostname, e.g. `app1.example.com` |
| `env_json` | JSON | Build-time env vars (optional v1) |
| `webhook_secret` | string | HMAC secret for GitHub webhook |
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
/data/
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
| `ADMIN_DOMAIN` (config) | Admin UI + API |
| `{project.domain}` | Serve files from `/data/sites/{project_id}/current` |
| Unknown host | 404 plain text |

**Static serving behavior (match Cloudflare Pages basics):**

- Try exact file path
- Fall back to `index.html` for directories
- SPA fallback (optional per-project flag): unknown paths → `index.html`

---

## GitHub Integration

### Authentication (Admin UI)

- GitHub OAuth App with scopes: `read:user`, `repo` (or `public_repo` if all repos are public)
- Callback URL: `https://{ADMIN_DOMAIN}/auth/github/callback`
- Session cookie (HttpOnly, Secure when behind tunnel)

### Repo Selection

- `GET /api/github/repos` — list repos for authenticated user (paginated)
- Filter/search client-side or via `?q=`

### Webhooks (v1 — manual-friendly)

Two options; pick one for implementation:

| Approach | Pros | Cons |
|---|---|---|
| **A. Per-project webhook URL + secret** | Simple; no GitHub App | User adds webhook in repo settings |
| **B. GitHub App** | Auto webhooks, org support | More setup for self-hosters |

**Recommendation for v1:** Approach A.

- Webhook URL: `https://{ADMIN_DOMAIN}/webhooks/github/{project_id}`
- Events: `push`
- Verify `X-Hub-Signature-256` with project `webhook_secret`
- On push to configured branch → enqueue build

---

## Build Pipeline

```
Webhook / manual trigger
        │
        ▼
  Enqueue deployment (status: queued)
        │
        ▼
  Worker picks job (single concurrent build in v1)
        │
        ├── git clone --depth 1 --branch {branch} (or fetch + checkout)
        ├── cd {root_dir} if set
        ├── inject env vars
        ├── run build_command in shell (bash -lc)
        ├── verify output_dir exists and is non-empty
        ├── rsync/cp to releases/{deployment_id}/
        ├── atomic symlink swap
        └── status: success | failed + log
```

**Build environment:**

- v1: host OS toolchain (Node, etc. installed on the machine)
- v1.1 (optional): Docker runner with configurable image per project

**Concurrency:** One build at a time globally in v1 to avoid CPU thrashing on a Mac Mini. Queue additional jobs.

**Manual redeploy:** `POST /api/projects/{id}/deploy` — rebuild latest commit on branch.

---

## API (v1)

All `/api/*` routes require session auth except webhooks.

| Method | Path | Description |
|---|---|---|
| GET | `/auth/github` | Start OAuth |
| GET | `/auth/github/callback` | OAuth callback |
| POST | `/auth/logout` | End session |
| GET | `/api/me` | Current user |
| GET | `/api/github/repos` | List repos |
| GET | `/api/projects` | List projects |
| POST | `/api/projects` | Create project |
| GET | `/api/projects/{id}` | Get project |
| PATCH | `/api/projects/{id}` | Update project |
| DELETE | `/api/projects/{id}` | Delete project + files |
| POST | `/api/projects/{id}/deploy` | Trigger manual build |
| GET | `/api/projects/{id}/deployments` | List deployments |
| GET | `/api/deployments/{id}` | Deployment detail |
| GET | `/api/deployments/{id}/logs` | Stream or return build log |
| POST | `/webhooks/github/{project_id}` | GitHub push webhook |

---

## Admin UI (v1 screens)

1. **Login** — “Sign in with GitHub”
2. **Dashboard** — list projects, last deploy status, domain link
3. **New project wizard**
   - Select repo (dropdown)
   - Branch (default `main`)
   - Build command (default suggestions by framework detection — optional nice-to-have)
   - Output directory (default `dist` or `public`)
   - Domain hostname
   - Show webhook URL + secret to paste in GitHub
4. **Project detail** — edit settings, deployment history, log viewer, “Deploy now”
5. **Settings** — admin domain, data directory (read-only display)

Keep UI minimal: forms + tables, no design system required for v1.

---

## Configuration

Environment variables or `/etc/selfpages/config.yaml`:

```yaml
listen_addr: ":8080"
data_dir: "/var/lib/selfpages"
admin_domain: "pages-admin.example.com"
github:
  client_id: "..."
  client_secret: "..."
session_secret: "random-32-bytes"
```

Secrets should come from env vars in production, not committed to disk in plain text if avoidable.

---

## Cloudflare Tunnel Setup

Document in README; not built into the app.

Example `config.yml` for `cloudflared`:

```yaml
tunnel: <tunnel-id>
credentials-file: /path/to/credentials.json

ingress:
  - hostname: pages-admin.example.com
    service: http://localhost:8080
  - hostname: app1.example.com
    service: http://localhost:8080
  - hostname: app2.example.com
    service: http://localhost:8080
  - service: http_status:404
```

**Wildcard option:** If all hostnames share a pattern, a single wildcard ingress rule can forward `*.example.com` to `:8080` and let the app route by `Host` — simpler tunnel config.

```yaml
ingress:
  - hostname: "*.example.com"
    service: http://localhost:8080
  - service: http_status:404
```

---

## Security Considerations (v1)

- Validate GitHub webhook signatures
- Session cookies: HttpOnly, SameSite=Lax
- Build commands are user-defined — **trusted admin only**; document that this is arbitrary code execution on the host
- Sanitize `domain` to prevent header injection (alphanumeric, dots, hyphens only)
- Rate-limit webhook endpoint (basic)
- Do not expose `/data` paths via HTTP

---

## Installation (v1)

**Option A — binary + systemd**

```bash
curl -fsSL .../install.sh | bash
# installs binary to /usr/local/bin/selfpages
# creates systemd unit
# creates /var/lib/selfpages
```

**Option B — Docker Compose**

```yaml
services:
  selfpages:
    image: selfpages:latest
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
    environment:
      - ADMIN_DOMAIN=pages-admin.example.com
      - GITHUB_CLIENT_ID=...
      - GITHUB_CLIENT_SECRET=...
      - SESSION_SECRET=...
```

Recommend shipping both; Docker for easy trials, binary for Mac Mini bare-metal.

---

## Implementation Plan

### Milestone 0 — Scaffold (1–2 days)

- [ ] Repo structure, CI (lint + test)
- [ ] Config loading, SQLite migrations
- [ ] HTTP server skeleton with health check `GET /healthz`

### Milestone 1 — Routing + Static Serve (2–3 days)

- [ ] Host-based router
- [ ] Project CRUD in DB (API only, no auth yet)
- [ ] Serve static files from `sites/{id}/current`
- [ ] SPA fallback flag
- [ ] Seed/deploy test fixture site manually

### Milestone 2 — GitHub Auth + UI (2–3 days)

- [ ] OAuth flow + sessions
- [ ] List GitHub repos
- [ ] Minimal admin UI (dashboard + create project)
- [ ] Protect API routes

### Milestone 3 — Build Worker (3–4 days)

- [ ] Deployment queue (in-process channel + single worker)
- [ ] Git clone, run build, publish to releases/
- [ ] Atomic symlink swap
- [ ] Build logs to file + API
- [ ] Manual “Deploy now”

### Milestone 4 — Webhooks (1–2 days)

- [ ] Webhook endpoint + signature verification
- [ ] Branch filter on push events
- [ ] UI shows webhook URL + secret + setup instructions

### Milestone 5 — Polish + Ship (2–3 days)

- [ ] Install script + systemd unit
- [ ] Docker Compose + Dockerfile
- [ ] README: tunnel setup, GitHub OAuth app creation, first deploy walkthrough
- [ ] Basic error handling and log viewer in UI

**Total estimate:** ~2 weeks part-time for one developer.

---

## Testing Strategy

| Layer | Approach |
|---|---|
| Unit | Router matching, webhook signature verification, config validation |
| Integration | Build pipeline with a tiny fixture repo (HTML only, no-op build) |
| E2E | Optional: spin up service, curl Host header, assert 200 |

Fixture repo for CI:

```bash
# build_command: "echo built > dist/index.html"
# output_dir: dist
```

---

## Open Questions

1. **Project name?** Working title: `selfpages` (change freely).
2. **OAuth vs GitHub App for v1?** Spec recommends OAuth + manual webhooks.
3. **Build isolation:** Accept host subprocess for v1, or require Docker from day one?
4. **Monorepo support:** Is `root_dir` enough for v1?
5. **Custom 404 pages:** Per-project `404.html` — include in v1 or defer?

---

## Success Criteria (v1 Done)

- [ ] Install on Linux with one command
- [ ] Sign in with GitHub
- [ ] Create two projects with different domains
- [ ] Push to branch → automatic build → site updates
- [ ] Cloudflare Tunnel wildcard sends traffic to both sites correctly
- [ ] Build logs visible in admin UI
- [ ] Failed build does not take down previous successful deployment

---

## Future (v2+)

- Preview URLs per branch (`branch---project.example.com`)
- Rollback to previous deployment
- GitHub App for automatic webhook management
- Docker-based build images per project
- Environment variables UI + secrets encryption
- Build concurrency limits per project
- Metrics (Prometheus)
- Email/Slack notifications on deploy failure
