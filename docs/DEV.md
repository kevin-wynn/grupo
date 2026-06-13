# Local Development & Testing Plan

How to run Grupo on your machine, iterate quickly, and verify deploys end-to-end before shipping to the Mac Mini.

**Goal:** change code → see it in the browser in seconds; change a site → see a new deploy in under a minute.

---

## Overview

Local dev mirrors production with three differences:

| Production | Local dev |
|---|---|
| systemd service | `make dev` (foreground processes) |
| `/var/lib/grupo` | `./.data/` in repo (gitignored) |
| Real ingress + TLS | `/etc/hosts` + plain HTTP on `:8080` |
| GitHub App webhooks from GitHub | Tunnel for full flow, or fixture/manual triggers for fast loop |

```
Browser ──► admin.mygrupo.dev:8080 ──► Grupo router
                                         ├── /api/*     → Go API
                                         ├── admin host → React (Vite HMR in dev)
                                         └── app1.*     → ./.data/sites/.../current
```

---

## Prerequisites

Install once on your dev machine (Linux or macOS with Docker Desktop):

| Tool | Version | Why |
|---|---|---|
| Go | 1.22+ | API, router, build worker |
| Node.js | 20+ | React / Vite frontend |
| Docker | 24+ | Build containers (same as prod) |
| Make | any | Dev commands |
| Git | any | Clone + fixture repos |

Optional but useful:

| Tool | Why |
|---|---|
| [air](https://github.com/air-verse/air) | Go hot reload |
| [Caddy](https://caddyserver.com/) | Optional local reverse proxy (closer to prod ingress) |
| [ngrok](https://ngrok.com/) or similar | Expose localhost for GitHub OAuth + webhooks |

---

## One-time setup

### 1. Clone and install frontend deps

```bash
git clone https://github.com/kevin-wynn/grupo.git
cd grupo
make setup    # go mod download + npm install in web/
```

### 2. Local hostnames

Add to `/etc/hosts` so routing matches production hostnames:

```
127.0.0.1 admin.mygrupo.dev
127.0.0.1 app1.mygrupo.dev
127.0.0.1 app2.mygrupo.dev
```

Open the admin UI at `http://admin.mygrupo.dev:8080`.

### 3. Dev config

Copy the example env file:

```bash
cp .env.example .env.local
```

`.env.local` (planned defaults):

```bash
GRUPO_LISTEN_ADDR=:8080
GRUPO_DATA_DIR=./.data
GRUPO_ADMIN_DOMAIN=admin.mygrupo.dev
GRUPO_DEV=true

# GitHub App — see "GitHub modes" below
GITHUB_APP_ID=
GITHUB_CLIENT_ID=
GITHUB_CLIENT_SECRET=
GITHUB_WEBHOOK_SECRET=
GITHUB_PRIVATE_KEY_PATH=./.secrets/github-app.pem
GRUPO_SESSION_SECRET=dev-session-secret-change-me
```

Create `.data/` and `.secrets/` (both gitignored):

```bash
mkdir -p .data .secrets
```

### 4. Docker access

The dev server runs builds the same way as production — it needs the Docker socket:

```bash
docker info   # should succeed without sudo
```

On Linux, your user must be in the `docker` group.

---

## Daily dev loop

### Start everything

```bash
make dev
```

Planned behavior:

| Process | Port | Reload |
|---|---|---|
| Go API + hostname router | `:8080` | `air` restarts on Go file changes |
| Vite (React UI) | `:5173` | HMR in browser |

In dev mode, requests to `admin.mygrupo.dev:8080` proxy non-API paths to Vite so you get instant UI updates. API calls go directly to Go on `:8080`.

**Frontend-only work:** edit files in `web/src/` → browser updates immediately.

**Backend-only work:** edit files in `internal/` → `air` rebuilds → refresh browser.

**Full stack:** `make build` to verify production embed path still works.

### Stop

`Ctrl+C` in the terminal running `make dev`.

---

## GitHub modes

GitHub App OAuth and webhooks require a public HTTPS URL. Use two modes depending on what you're working on.

### Mode A — Fast loop (no GitHub)

Use for router, static serving, Docker builds, and UI work.

```bash
GRUPO_DEV=true
GRUPO_SKIP_GITHUB_AUTH=true   # planned flag
make dev
make seed                      # creates fixture project in SQLite
make deploy-fixture            # triggers build without a webhook
```

What `make seed` sets up (planned):

- Project **fixture-hello** → domain `app1.mygrupo.dev`
- Points at `fixtures/hello-site/` (local directory, not GitHub)
- Build image `alpine:3.20`, command writes `dist/index.html`

Visit `http://app1.mygrupo.dev:8080` after deploy completes.

**Fastest edit loop:**

```bash
# terminal 1
make dev

# terminal 2 — after editing fixtures/hello-site/
make deploy-fixture
# refresh app1.mygrupo.dev:8080
```

No push, no webhook, no tunnel — useful for 90% of backend/UI iteration.

### Mode B — Full GitHub integration

Use when working on OAuth, repo listing, webhook ingestion, or clone-with-token.

1. Create a **development GitHub App** (separate from production).
2. Start a tunnel to your machine:

```bash
ngrok http 8080
# e.g. https://abc123.ngrok-free.app
```

3. Set GitHub App URLs to the tunnel:

| Setting | Value |
|---|---|
| Callback URL | `https://abc123.ngrok-free.app/auth/github/callback` |
| Webhook URL | `https://abc123.ngrok-free.app/webhooks/github` |

4. Update `.env.local` with the App credentials.
5. `make dev` and sign in at `http://admin.mygrupo.dev:8080`.
6. Create a project pointing at a **test repo** you control.
7. Push to the configured branch → webhook → build → site updates.

**Tip:** keep a dedicated throwaway repo (e.g. `grupo-test-site`) with a 5-second build for webhook testing.

---

## Fixture repos

Ship two fixtures in the repo for deterministic testing:

```
fixtures/
├── hello-site/          # minimal static output
│   ├── build.sh         # writes dist/index.html
│   └── dist/            # committed placeholder (optional)
└── node-site/           # optional: tiny Vite/React app for real Node builds
    ├── package.json
    └── ...
```

### `fixtures/hello-site`

Used by `make seed` and CI. Build command:

```bash
mkdir -p dist && echo "hello from fixture" > dist/index.html
```

### `fixtures/node-site` (optional, Milestone 3+)

Validates real `node:22-bookworm` builds:

```bash
npm ci && npm run build
```

---

## Makefile targets (planned)

| Command | Purpose |
|---|---|
| `make setup` | Install Go + npm dependencies |
| `make dev` | Start Go (air) + Vite with dev proxy |
| `make build` | Production build (embed frontend → binary) |
| `make test` | Unit tests (`go test ./...`) |
| `make test-integration` | Docker build pipeline tests |
| `make test-web` | Frontend unit tests (Vitest) |
| `make seed` | Insert fixture project into `./.data` |
| `make deploy-fixture` | Trigger build for fixture project |
| `make smoke` | curl-based health + routing checks |
| `make webhook-test` | POST signed fake GitHub push payload |
| `make clean` | Remove `./.data` and build artifacts |

---

## Manual smoke checklist

Run after meaningful changes or before opening a PR.

### 1. Health

```bash
curl -sf http://127.0.0.1:8080/healthz
```

### 2. Admin UI loads

Open `http://admin.mygrupo.dev:8080` — login page or dashboard renders.

### 3. Hostname routing

After `make seed && make deploy-fixture`:

```bash
curl -sf -H "Host: app1.mygrupo.dev" http://127.0.0.1:8080/
# expect: hello from fixture

curl -sf -o /dev/null -w "%{http_code}" -H "Host: unknown.mygrupo.dev" http://127.0.0.1:8080/
# expect: 404
```

### 4. SPA fallback (if enabled on project)

```bash
curl -sf -H "Host: app1.mygrupo.dev" http://127.0.0.1:8080/some/client/route
# expect: index.html contents when spa_fallback=true
```

### 5. Build logs

```bash
curl -sf -H "Host: admin.mygrupo.dev" http://127.0.0.1:8080/api/projects
# grab deployment id, then:
curl -sf -H "Host: admin.mygrupo.dev" http://127.0.0.1:8080/api/deployments/{id}/logs
```

### 6. Failed build keeps previous deploy

```bash
# temporarily set build_command to "exit 1"
make deploy-fixture
curl -sf -H "Host: app1.mygrupo.dev" http://127.0.0.1:8080/
# expect: previous successful output still served
```

---

## Automated testing plan

### Unit tests (every commit)

Fast, no Docker. Target packages:

| Package | Examples |
|---|---|
| `internal/router` | Host → handler mapping, unknown host 404 |
| `internal/config` | Env parsing, required fields |
| `internal/github` | Webhook signature verification |
| `internal/build` | Path validation, output dir checks |

```bash
make test
```

### Integration tests (PR + pre-push)

Requires Docker. Runs in CI and locally:

```bash
make test-integration
```

Flow:

1. Create temp data dir
2. Insert fixture project via test helper
3. Run build worker against `fixtures/hello-site`
4. Assert files land in `sites/{id}/current`
5. Assert HTTP server returns expected body via Host header

Uses `testing` + `testcontainers` or direct Docker CLI — keep to one happy path + one failure path in v1.

### Webhook tests (no tunnel)

```bash
make webhook-test
```

Builds a signed `push` payload from `fixtures/webhook-push.json`, POSTs to `/webhooks/github`, asserts deployment queued. No real GitHub needed.

### Frontend tests (optional v1)

Vitest + React Testing Library for:

- New project form validation
- Deployment status badge rendering

```bash
make test-web
```

### End-to-end (manual until v2)

| Scenario | Mode |
|---|---|
| Sign in with GitHub | B (tunnel) |
| Create project + auto webhook | B |
| Push → deploy | B |
| Edit fixture → redeploy | A |
| Two projects, two hostnames | A |

---

## Recommended workflow by task

| You're working on… | Setup | Verify with… |
|---|---|---|
| React UI / Tailwind | `make dev` | Browser HMR on admin host |
| Hostname router / static serve | `make dev` + Mode A | `make smoke` |
| Docker build pipeline | `make dev` + Mode A | `make deploy-fixture` |
| GitHub OAuth / repo list | Mode B + tunnel | Sign in, list repos in UI |
| Webhooks | Mode B + tunnel | Push to test repo |
| Production binary | `make build` + run binary | Same smoke checks on `:8080` |

---

## Simulating production locally

Optional step before deploying to the Mac Mini:

```bash
make build
GRUPO_DATA_DIR=./.data GRUPO_ADMIN_DOMAIN=admin.mygrupo.dev ./bin/grupo serve
```

Or install the systemd unit on a local Linux VM with the same `.env` values — validates permissions, Docker socket, and data dir paths without touching prod.

---

## CI pipeline (planned)

Runs on every PR:

```yaml
jobs:
  test:
    - make setup
    - make test
    - make test-web        # when frontend tests exist
  integration:
    - make test-integration  # requires Docker runner
  build:
    - make build             # ensures embed + compile succeed
```

Same fixture repo and smoke curls as local dev — CI should be runnable with `make ci` locally.

---

## Gitignored paths

```
.data/
.secrets/
web/node_modules/
web/dist/
bin/
tmp/
.env.local
```

---

## Quick reference: fastest path to "see it working"

Once Milestone 1–3 land:

```bash
git clone ... && cd grupo
make setup
# add /etc/hosts entries
cp .env.example .env.local
echo "GRUPO_SKIP_GITHUB_AUTH=true" >> .env.local

make dev
# new terminal:
make seed && make deploy-fixture

open http://app1.mygrupo.dev:8080      # deployed site
open http://admin.mygrupo.dev:8080     # admin UI
```

Edit `fixtures/hello-site/build.sh`, run `make deploy-fixture` again, refresh — that's the core loop.

---

## Open items (to implement with Milestone 0)

- [ ] `.env.example` with all dev vars documented
- [ ] `make dev` script (Procfile, `mage`, or shell wrapper)
- [ ] `GRUPO_DEV` + `GRUPO_SKIP_GITHUB_AUTH` flags in config
- [ ] `fixtures/hello-site` + `make seed` / `make deploy-fixture`
- [ ] `fixtures/webhook-push.json` + `make webhook-test`
- [ ] `make smoke` curl script
- [ ] CI workflow running `make test` + `make test-integration`
