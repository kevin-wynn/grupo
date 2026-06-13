.PHONY: setup dev build test test-integration test-web seed deploy-fixture smoke webhook-test clean ci install

GO ?= go
NPM ?= npm
BIN := bin/grupo
WEB_DIR := web

setup:
	$(GO) mod download
	cd $(WEB_DIR) && $(NPM) install

dev:
	@set -a && [ -f .env.local ] && . ./.env.local; set +a; \
	$(MAKE) -j2 dev-api dev-web

dev-api:
	@set -a && [ -f .env.local ] && . ./.env.local; set +a; \
	GRUPO_DEV=true $(GO) run ./cmd/grupo serve

dev-web:
	cd $(WEB_DIR) && $(NPM) run dev

build: web-build
	@mkdir -p bin
	$(GO) build -o $(BIN) ./cmd/grupo

web-build:
	cd $(WEB_DIR) && $(NPM) run build
	rm -rf internal/webembed/dist
	cp -r $(WEB_DIR)/dist internal/webembed/dist

test:
	$(GO) test ./cmd/... ./internal/...

test-integration:
	$(GO) test ./internal/build/... -tags=integration

test-web:
	cd $(WEB_DIR) && $(NPM) run build

seed:
	@set -a && [ -f .env.local ] && . ./.env.local; set +a; \
	$(GO) run ./cmd/grupo seed

deploy-fixture:
	@set -a && [ -f .env.local ] && . ./.env.local; set +a; \
	$(GO) run ./cmd/grupo deploy-fixture

smoke:
	@./scripts/smoke.sh

webhook-test:
	@./scripts/webhook-test.sh

clean:
	rm -rf bin tmp $(WEB_DIR)/dist ./.data

ci: setup web-build test build

install: build
	install -d /usr/local/bin /etc/grupo /var/lib/grupo
	install -m 0755 $(BIN) /usr/local/bin/grupo
	install -m 0644 deploy/grupo.service /etc/systemd/system/grupo.service
	install -m 0644 deploy/config.example.yaml /etc/grupo/config.example.yaml
