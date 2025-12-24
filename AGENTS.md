# AGENTS.md — golang_simple_server

This document provides operational guidance for coding agents (Codex, Copilot, etc.) working in this repository.

## Project overview

- Repository: `spcent/golang_simple_server`
- Language: Go (module-based, see `go.mod`)
- Entry point: `main.go`
- Common folders (verify locally):
  - `handlers/` — HTTP handlers (request/response boundary)
  - `pkg/` — reusable packages (routing/middleware/config/etc.)
  - `docs/` — design notes / usage docs
  - `scripts/` — helper scripts (dev/build/release)
  - `.github/workflows/` — CI workflows
- Env template: `env.example`

**Server Purpose & Capabilities**

This server provides a minimal, production-grade web application runtime built exclusively on the Go standard library.
Its primary purpose is to offer a clean, dependency-free foundation for building RESTful APIs, webhook receivers, and real-time services with predictable behavior and long-term maintainability.
The server includes an efficient HTTP router with middleware support, structured request context handling, and consistent JSON response semantics.
It supports secure authentication via JWT, reliable webhook verification and deduplication, and an in-process pub-sub system for event-driven workflows.
Real-time communication is enabled through a lightweight WebSocket implementation with connection management and backpressure controls.
For small to medium workloads, it provides a built-in local persistence layer suitable for tokens, deduplication, and configuration data.
The server emphasizes observability, graceful shutdown, and explicit extensibility while avoiding external dependencies or hidden runtime behavior.

## Golden rules (must follow)

1. Make changes that are minimal, explicit, and testable.
2. Do not change public behavior without updating tests and docs.
3. Never introduce new external dependencies unless explicitly requested.
4. Do not log secrets/credentials/PII. Redact tokens, Authorization headers, cookies, and user identifiers.
5. Prefer Go standard library patterns and idiomatic Go (context-aware, error-first, small packages).

## Local setup

### Prerequisites
- Go toolchain installed (version should satisfy `go.mod`).
- Optional: `make` (if you use `Makefile` targets).

### Environment
- Copy env template and fill values:
  - `cp env.example .env` (or follow project’s actual loader)

## Build / run / test (authoritative commands)

Agents must discover the actual commands by inspecting `Makefile` and CI.
Use the following order:

1. List available Make targets:
   - `make help` (if present) or just `cat Makefile`
2. Try standard Go commands:
   - `go test ./...`
   - `go test -race ./...` (if CI budget allows)
   - `go vet ./...`
   - `gofmt -w .`

If the repo includes a canonical target set, prefer it:
- `make test`
- `make lint`
- `make fmt`
- `make run`

## Code organization conventions

### Handlers
- Handlers should be thin:
  - Parse/validate input
  - Call domain/service logic
  - Map errors to HTTP status codes consistently
- Avoid business logic in handlers; move it under `pkg/` (or a dedicated internal layer if you add one).

### Packages under `pkg/`
- Keep packages cohesive; avoid circular imports.
- Prefer small interfaces at boundaries, concrete structs internally.
- Context propagation:
  - All request-scoped operations accept `context.Context`.
  - Timeouts/cancellation should be respected.

### Error handling & responses
- Do not panic for expected errors.
- Use consistent error typing/wrapping (`fmt.Errorf("...: %w", err)`).
- Centralize HTTP error mapping if possible (middleware or helper).

## Testing guidelines

- Prefer table-driven tests.
- For HTTP:
  - Use `net/http/httptest` to test handlers and middleware.
  - Avoid binding to real ports in unit tests.
- If the project has integration tests:
  - Keep them under a separate package or tag (e.g. `-tags=integration`).

### What to update when changing behavior
- Update/extend tests first (or alongside change).
- Update `README.md` and/or `docs/` for:
  - new env vars
  - new routes/endpoints
  - new Make targets or scripts

## Security & safety checklist (before finishing)

- [ ] No secrets in code or logs
- [ ] Auth middleware is applied to protected routes
- [ ] Input validation for JSON/body/query/path params exists
- [ ] Rate limiting / request size limits are considered (if exposed publicly)
- [ ] Timeouts set on server and outbound calls
- [ ] No unsafe deserialization or shell exec without justification

## Review guidelines for agents

When submitting changes (PR or patch), include:
- Summary of change
- Why it’s needed
- How to test (exact commands)
- Risk assessment (what could break, compatibility notes)

If you modify routing/middleware, explicitly list:
- affected endpoints
- auth requirements
- backward compatibility

## Directory-specific overrides

Agents may add more specific rules in subdirectories using `AGENTS.override.md`
(e.g., for `handlers/` or `pkg/`), keeping root guidance stable.

