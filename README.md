# Grupo

Self-hostable static site deployment for Linux. Connect GitHub, configure domains, and serve multiple static sites from one instance.

**Stack:** Go backend (systemd binary) · React + Tailwind UI · Docker builds · GitHub App

**Domain:** `mygrupo.dev` (admin at `admin.mygrupo.dev`)

See [docs/SPEC.md](docs/SPEC.md) for the full product spec and [docs/DEV.md](docs/DEV.md) for local development.

---

## Quick start (local dev)

```bash
make setup
cp .env.example .env.local

# /etc/hosts:
# 127.0.0.1 admin.mygrupo.dev app1.mygrupo.dev

make build          # embed React UI + compile binary
./bin/grupo serve   # or: make dev

# new terminal
make deploy-fixture

open http://app1.mygrupo.dev:8080      # deployed site
open http://admin.mygrupo.dev:8080     # admin UI
```

With `GRUPO_SKIP_GITHUB_AUTH=true` in `.env.local`, the API works without GitHub OAuth — ideal for router, UI, and fixture builds.

---

## Prerequisites

| Tool | Version |
|---|---|
| Go | 1.22+ |
| Node.js | 20+ |
| Docker | 24+ (production builds; fixture deploy falls back to local shell when Docker is unavailable in dev) |
| Make | any |

---

## Makefile targets

| Command | Purpose |
|---|---|
| `make setup` | Install Go + npm dependencies |
| `make dev` | Start Go API + Vite (HMR) |
| `make build` | Production build (embed frontend → binary) |
| `make test` | Unit tests |
| `make seed` | Insert fixture project into `./.data` |
| `make deploy-fixture` | Build and publish fixture site to `app1.mygrupo.dev` |
| `make smoke` | curl-based health + routing checks |
| `make webhook-test` | POST signed fake GitHub push payload |
| `make ci` | setup + test + build |

---

## Configuration

Copy `.env.example` to `.env.local` for development, or use `/etc/grupo/config.yaml` in production:

```yaml
listen_addr: ":8080"
data_dir: "/var/lib/grupo"
admin_domain: "admin.mygrupo.dev"
session_secret: "random-32-bytes"
github:
  app_id: "123456"
  client_id: "Iv1.xxxx"
  client_secret: "..."
  webhook_secret: "..."
  private_key_path: "/etc/grupo/github-app.pem"
```

Environment variables override file values (`GRUPO_*`, `GITHUB_*`).

---

## GitHub App setup

1. Create a GitHub App at https://github.com/settings/apps/new
2. Callback URL: `https://{ADMIN_DOMAIN}/auth/github/callback`
3. Webhook URL: `https://{ADMIN_DOMAIN}/webhooks/github`
4. Permissions: repo metadata (read), contents (read), webhooks (read/write), administration (read)
5. Subscribe to `push`, `installation`, `installation_repositories`
6. Generate a private key → save as `/etc/grupo/github-app.pem`
7. Paste app ID, client ID/secret, and webhook secret into config

---

## Production install

```bash
make build
sudo ./scripts/install.sh
sudo systemctl start grupo
```

Place a reverse proxy in front to forward `*.mygrupo.dev` to `localhost:8080` with the original `Host` header preserved. Example (Caddy):

```
*.mygrupo.dev {
    reverse_proxy localhost:8080
}
```

---

## Architecture

- **Router** — single HTTP listener; routes by `Host` to admin UI or static sites
- **Control plane** — REST API, GitHub App auth, project CRUD, webhooks
- **Build worker** — clone repo, Docker build, atomic symlink deploy
- **Storage** — SQLite + filesystem under `/var/lib/grupo`

---

## Project layout

```
cmd/grupo/           # main entrypoint
internal/            # api, router, build, github, db, config
web/                 # React + Tailwind (Vite)
fixtures/            # hello-site fixture for dev/CI
scripts/             # install.sh, smoke.sh, webhook-test.sh
deploy/              # systemd unit + example config
```
