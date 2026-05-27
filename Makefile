.PHONY: help dev watch ensure-air test test-v lint fmt cover build run clean web-install web-dev

GOPATH_BIN := $(shell go env GOPATH)/bin
AIR_BIN    ?= $(shell command -v air 2>/dev/null || echo $(GOPATH_BIN)/air)

help:
	@echo "make dev         - run the Go backend with go run"
	@echo "make web-dev     - run the Vite dev server for the frontend"
	@echo "make watch       - live-reload: air (Go :8080) + Vite (:5173) en paralelo"
	@echo "make test        - run Go tests"
	@echo "make test-v      - run Go tests with -v"
	@echo "make cover       - run tests with coverage report"
	@echo "make lint        - run golangci-lint over the whole module"
	@echo "make fmt         - apply gofmt + goimports"
	@echo "make build       - build the single binary (requires web/dist)"
	@echo "make web-install - npm install in web/"
	@echo "make clean       - remove binaries and dist"

dev:
	go run ./cmd/modos-cpu

# Live-reload de desarrollo: air rebuildea el binario Go al cambiar *.go y
# Vite hace HMR para Vue/TS. Vite proxea /api y /ws al backend en :8080,
# así que durante dev se abre http://localhost:5173.
ensure-air:
	@if [ ! -x "$(AIR_BIN)" ]; then \
		echo ">> air no encontrado, instalando en $(GOPATH_BIN)..."; \
		GOFLAGS= go install github.com/air-verse/air@latest; \
	fi

watch: ensure-air
	@echo ">> watch: Go (air) :8080 + Vite :5173 — abrí http://localhost:5173"
	@( "$(AIR_BIN)" -c .air.toml 2>&1 | sed -u 's/^/[go]  /' ) & go_pid=$$!; \
	( cd web && npm run dev 2>&1 | sed -u 's/^/[web] /' ) & web_pid=$$!; \
	trap 'kill $$go_pid $$web_pid 2>/dev/null; wait 2>/dev/null' INT TERM; \
	wait $$go_pid $$web_pid

test:
	go test ./...

test-v:
	go test -v ./...

cover:
	go test -cover -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	@if command -v goimports >/dev/null 2>&1; then goimports -w -local github.com/jeanmarcos/modos-cpu .; fi

build: web-build
	go build -ldflags='-s -w' -o bin/modos-cpu ./cmd/modos-cpu

web-build:
	cd web && npm run build

run: build
	./bin/modos-cpu

web-install:
	cd web && npm install

web-dev:
	cd web && npm run dev

clean:
	rm -rf bin/ web/dist/ web/node_modules/ internal/staticui/dist/

# Admin helpers. The admin panel at /admin is disabled unless these three env
# vars are set when the binary boots: ADMIN_USER, ADMIN_PASS_HASH, SESSION_SECRET.

admin-hash:
	@if [ -z "$(PASS)" ]; then echo "usage: make admin-hash PASS=mypassword"; exit 1; fi
	@go run ./cmd/modos-cpu admin-hash -pass='$(PASS)'

session-secret:
	@openssl rand -hex 32
