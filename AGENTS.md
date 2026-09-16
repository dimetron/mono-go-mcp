# AGENTS.md

Agent instructions for `mono-go-mcp`.

## What this is

An MCP (Model Context Protocol) server, written in Go, that exposes the
[monobank open API](https://api.monobank.ua/docs/index.html) (spec
v250818, OpenAPI 3.0.3) as MCP tools.

- Module path: `github.com/dimetron/mono-go-mcp`
- MCP SDK: `github.com/modelcontextprotocol/go-sdk` (official Go SDK)
- Transport: stdio (default). Server name: `mono-go-mcp`.

## Commands

Use [Task](https://taskfile.dev/) (`brew install go-task`) — `taskfile.dev` Taskfile.yml is the task runner of record:

```sh
task build     # → bin/mono-go-mcp + bin/mono-go-cli (incremental via sources/generates)
task install   # build + copy both into $GOPATH/bin
task check     # gofmt -l gate, go vet, go build ./...
task fmt       # gofmt -w .
task smoke       # run the CLI (cmd/mono-go-cli) against the real API; alias: task cli
task run         # run the server locally (reads .env)
task clean       # rm -rf bin/
task upgrade     # go get -u ./... && go mod tidy
task release:check  # goreleaser check (.goreleaser.yaml validation)
task release:build  # cross-build all targets into dist/ (dry run)
task release:clean  # rm -rf dist/
```

Raw go equivalents (when task is unavailable):

```sh
go build ./...        # build all packages
go vet ./...          # static checks
go run ./cmd/mono-go-cli    # CLI: rates, sync, client info, two statement tables
go build -o bin/mono-go-mcp ./cmd/mono-go-mcp   # build the server binary
go build -o bin/mono-go-cli ./cmd/mono-go-cli   # build the CLI binary
gofmt -w .            # format (run before committing)
```

There is one test file (`internal/monoapi/cache_test.go`); `go test ./...`
runs it.

## cmd/mono-go-cli (terminal client)

Direct client for the monoapi layer — no MCP involved. Default output:
rates table, bank-sync table, and with `MONO_TOKEN` client-info plus
two statement tables (1st of last month → 1st of this month, 1st of
this month → now). Flags select parts: `-rates`, `-sync`, `-info`,
`-stmt`, `-account ID`, `-webhook URL`, `-no-wait`, `-version`. With no
flags everything runs. On a 429 from the second statement call it
waits `X-Auth-Interval-Expires` (capped at 60 s) and retries once.

## Project structure

```
mono-go-mcp/
├── go.mod                  # module github.com/dimetron/mono-go-mcp
├── Taskfile.yml            # task runner: build, install, check, smoke (taskfile.dev)
├── AGENTS.md               # this file
├── README.md               # user-facing docs + client config
├── docs/
│   ├── api.md              # monobank API reference (endpoints, schemas, limits)
│   └── api-spec.json       # extracted upstream OpenAPI 3.0.3 spec (source of truth)
├── bin/                    # build output (git-ignored)
├── cmd/
│   ├── mono-go-mcp/        # server entrypoint: .env -> client -> tools -> stdio
│   └── mono-go-cli/        # terminal client: monoapi calls + table output + flags
├── .goreleaser.yaml        # release config: both binaries, 5 OS/arch targets
├── .github/workflows/
│   └── release.yml         # GitHub Actions: goreleaser on v* tags
└── internal/
    ├── monoapi/            # HTTP client for monobank (no MCP deps)
    │   ├── client.go       # Client, APIError (429/403 handling), X-Token header
    │   ├── endpoints.go    # one method per API endpoint
    │   ├── types.go        # request/response types mirroring OpenAPI schemas
    │   ├── cache.go        # 65 s TTL cache for rate-limited /personal/* responses
    │   └── util.go         # small helpers
    └── tools/              # MCP tool layer
        └── tools.go        # Register(): one tool per endpoint, SDK ToolHandlerFor
```

Dependency direction: `cmd/mono-go-cli` and `cmd/mono-go-mcp` →
`internal/tools` → `internal/monoapi`. The `monoapi` package must stay
MCP-free; the `tools` package must stay HTTP-free. `cmd/mono-go-cli`
skips `internal/tools` and calls `internal/monoapi` directly (it is a
plain API client, not an MCP peer).

## Environment

- `MONO_TOKEN` — personal token from https://api.monobank.ua/ .
  Required for `/personal/*` tools; public tools work without it.
- `MONO_BASE_URL` — optional API base override (tests/proxy).
- At startup the server loads the nearest `.env` from the CWD
  (`loadDotEnv` in `cmd/mono-go-mcp/main.go`, via `joho/godotenv`).
  A missing `.env` is fine; variables already set in the shell
  environment take precedence over `.env` values.
- `.env` is git-ignored; never commit it.

## MCP tools exposed

| Tool                  | monobank endpoint                              | Auth     | Rate limit        |
|-----------------------|------------------------------------------------|----------|-------------------|
| mono_currency_rates   | GET /bank/currency                             | public   | upstream 5-min cache |
| mono_bank_sync        | GET /bank/sync                                 | public   | —                 |
| mono_client_info      | GET /personal/client-info                      | X-Token  | 1 per 60 s        |
| mono_statement        | GET /personal/statement/{account}/{from}/{to}  | X-Token  | 1 per 60 s        |
| mono_set_webhook      | POST /personal/webhook                         | X-Token  | —                 |

## SDK conventions (go-sdk v1.8.x)

- Tools are added with `mcp.AddTool(server, &mcp.Tool{...}, handler)`.
- Handler type is `mcp.ToolHandlerFor[In, Out]` with **three** return
  values: `(*mcp.CallToolResult, Out, error)`. Return `nil` result and
  let the SDK pack the output into `StructuredContent` + text content.
- Returning a non-`jsonrpc.Error` error is converted by the SDK into a
  tool-level error (`IsError=true`); do not hand-roll error results.
- Input schemas are generated from the `In` struct via
  `json:"..."` + `jsonschema:"description"` tags.
- `mcp.ToolHandler` (non-generic, 2 returns) is the low-level form;
  prefer `ToolHandlerFor`.

## monobank API facts that matter

- All money amounts are **integer minimal currency units**
  (kopiykas/cents). Divide by 100 for major units.
- Statement window: max **2,682,000 s** (31 days + 1 h); `account=0`
  means the default account; omitting `to` means now.
- Rate limits: `/personal/client-info` and `/personal/statement` —
  once per **60 s**; 429 responses include `X-Auth-Interval-Expires`.
  The client (`internal/monoapi/cache.go`) caches these responses for
  **65 s** (TTL, mutex-guarded). Closed statement windows only —
  `to=0` (now) bypasses the cache. Errors are never cached.
- The API is for personal use only; corporate/centralized use requires
  the separate provider API. Do not add endpoints that would make this
  a shared service.
- Errors: `{"errorDescription": "..."}` with meaningful HTTP status
  (401/403 missing-bad token, 429 rate limit, 404 bad request data).

## Editing rules

- Change one thing per change; keep the monoapi/tools split clean.
- Any new endpoint: add the type in `types.go`, method in
  `endpoints.go`, tool in `tools.go` — in that order, building between
  steps.
- Update `docs/api.md` and `docs/api-spec.json` whenever the upstream
  spec changes (the spec is embedded in the docs page HTML as
  `__redoc_state`, since `/docs/openapi.json` returns 403).
- Never commit `.env` or a real token.

## Release

[GoReleaser](https://goreleaser.com) v2 config (`.goreleaser.yaml`) +
GitHub Actions (`.github/workflows/release.yml`):

- Trigger: push a tag `v*` (e.g. `git tag v0.2.0 && git push origin v0.2.0`),
  or run the workflow manually (`workflow_dispatch`).
- Targets: both binaries (mono-go-mcp, mono-go-cli) for linux
  amd64+arm64, darwin amd64+arm64, windows amd64 (tar.gz archives; zip
  for Windows; `checksums.txt`).
- Version is stamped into the binary via
  `-X main.version={{ .Version }}` (see `var version` in
  `cmd/mono-go-mcp/main.go` — keep it a `var` or the stamp silently
  stops working).
- Locally: `task release:check` to validate config,
  `task release:build` for a full dry-run into `dist/`.
- The goreleaser version pin in `release.yml` (`version: v2.18.1`)
  should match the local `goreleaser` CLI major.minor when upgraded.