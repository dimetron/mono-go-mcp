# mono-go-mcp

[![ci](https://github.com/dimetron/mono-go-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/dimetron/mono-go-mcp/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/dimetron/mono-go-mcp/graph/badge.svg)](https://codecov.io/gh/dimetron/mono-go-mcp)
[![govulncheck](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Fdimetron%2Fmono-go-mcp%2Fgh-pages%2Fbadges%2Fvuln-badge.json)](https://github.com/dimetron/mono-go-mcp/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/dimetron/mono-go-mcp.svg)](https://pkg.go.dev/github.com/dimetron/mono-go-mcp)

[English](#english) | [Українська](#українська)

## English

An MCP (Model Context Protocol) server that exposes the
[monobank open API](https://api.monobank.ua/docs/index.html) as tools.
Built with the official Go SDK
[`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk).

Module: `github.com/dimetron/mono-go-mcp`.

### Tools

| MCP tool              | monobank endpoint                              | auth    | notes                          |
|-----------------------|------------------------------------------------|---------|--------------------------------|
| `mono_currency_rates` | GET /bank/currency                             | public  | upstream 5-min cache           |
| `mono_bank_sync`      | GET /bank/sync                                 | public  | bank public key + server time  |
| `mono_client_info`    | GET /personal/client-info                      | token   | 1 req / 60 s                   |
| `mono_statement`      | GET /personal/statement/{account}/{from}/{to}  | token   | 1 req / 60 s, 31-day max window|
| `mono_set_webhook`    | POST /personal/webhook                         | token   | URL must answer GET → HTTP 200 |

All monetary values are integer minimal currency units (kopiykas/cents).

### Build

Uses [Task](https://taskfile.dev/) (`brew install go-task`):

```sh
task build     # bin/mono-go-mcp + bin/mono-go-cli
task install   # both into $GOPATH/bin
task check     # fmt gate + vet + build
task --list    # all tasks
```

### CI / PR merge gate

Every PR runs [`.github/workflows/ci.yml`](.github/workflows/ci.yml):
`go vet`, tests with coverage and **govulncheck**. Merge is gated on a
**90% code-coverage floor** and **zero called vulnerabilities**
(`MIN_COVERAGE` env at the top of the workflow). Coverage is published
to Codecov, the vulnerability count as a shields.io endpoint badge.

Or plain go:

```sh
go build ./... && go vet ./...
go build -o mono-go-mcp ./cmd/mono-go-mcp
```

### Run (stdio MCP server)

Get a personal token at <https://api.monobank.ua/> (personal cabinet).
Public tools work without it.

Either put the token in a `.env` file (copy `.env.example`) in the
directory the server runs from:

```sh
# .env
MONO_TOKEN="<your token>"
```

```sh
./mono-go-mcp
```

…or export it in the shell (takes precedence over `.env`):

```sh
export MONO_TOKEN="<your token>"
./mono-go-mcp
```

The server speaks MCP over stdio and is meant to be launched by an MCP
client. Example generic client config:

```json
{
  "mcpServers": {
    "mono-go-mcp": {
      "command": "/path/to/mono-go-mcp",
      "env": { "MONO_TOKEN": "<your token>" }
    }
  }
}
```

Optional env: `MONO_BASE_URL` overrides the API base (for tests).

At startup the server auto-loads `.env` from its working directory
(missing file is fine; shell env vars win over `.env` values).

### CLI (mono-go-cli)

A terminal client for the same monobank API — no MCP client needed.
With no flags it prints currency rates, bank sync info and, with
`MONO_TOKEN`, client info plus **two statement tables** for the default
account:

- **last month** — from the 1st of the previous month to the 1st of
  this month;
- **this month** — from the 1st of this month to now.

```sh
task smoke                       # or: go run ./cmd/mono-go-cli
mono-go-cli -stmt                # only the statement tables
mono-go-cli -info -stmt          # client info + statements
mono-go-cli -stmt -account <ID>  # specific account/jar (default "0")
mono-go-cli -rates               # only the rates table
mono-go-cli -sync                # only bank public key + server time
mono-go-cli -webhook <URL>       # set the statement webhook URL
mono-go-cli -no-wait             # fail on 429 instead of waiting
mono-go-cli -version
```

The two statement windows are separate `/personal/statement` calls, so
the second one may hit the 60 s rate limit; the CLI then waits
(`X-Auth-Interval-Expires`, capped at 60 s) and retries once. Use
`-no-wait` to disable that.

### Verify

### Docs

- [`docs/api.md`](docs/api.md) — monobank API reference (endpoints,
  schemas, rate limits) with references to the upstream docs.
- [`docs/api-spec.json`](docs/api-spec.json) — extracted upstream
  OpenAPI 3.0.3 spec (v250818), the source of truth for types.
- [`AGENTS.md`](AGENTS.md) — repo layout and agent instructions.

### Release

Binaries for Linux (amd64, arm64), macOS (amd64, arm64) and Windows
(amd64) are built by [GoReleaser](https://goreleaser.com) in a GitHub
Actions workflow (`.github/workflows/release.yml`) and published to
GitHub Releases with a `checksums.txt`.

To cut a release:

```sh
git tag v0.2.0
git push origin v0.2.0   # triggers the release workflow
```

The version is stamped into the binary (`main.version`). Local dry
run: `task release:build` (cross-builds both binaries into `dist/`),
validate config with `task release:check`.

## Structure

```
mono-go-mcp/
├── Taskfile.yml         # task runner (taskfile.dev): build, install, check
├── .goreleaser.yaml     # release config: both binaries, linux/darwin amd64+arm64, windows amd64
├── .github/workflows/   # release.yml: goreleaser on v* tags
├── cmd/mono-go-mcp/     # MCP server entrypoint: .env → client → tools → stdio
├── cmd/mono-go-cli/     # terminal client: monoapi calls → tables; part-selecting flags
├── docs/                # api.md + api-spec.json
└── internal/
    ├── monoapi/         # monobank HTTP client (no MCP deps)
    └── tools/           # MCP tool layer (no HTTP details)
```

### Notes

- The monobank personal API is for personal use only; do not point this
  server at other people's tokens (that requires the corporate API).
- Never commit `.env` or a real token.

---

## Українська

MCP-сервер (Model Context Protocol), який відкриває
[відкритий API monobank](https://api.monobank.ua/docs/index.html) як
інструменти. Побудований на офіційному Go SDK
[`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk).

Модуль: `github.com/dimetron/mono-go-mcp`.

### Інструменти

| MCP-інструмент        | Ендпоінт monobank                              | авторизація | примітки                            |
|-----------------------|------------------------------------------------|-------------|-------------------------------------|
| `mono_currency_rates` | GET /bank/currency                             | публічний   | кеш 5 хв на боці upstream           |
| `mono_bank_sync`      | GET /bank/sync                                 | публічний   | публічний ключ банку + час сервера  |
| `mono_client_info`    | GET /personal/client-info                      | токен       | 1 запит / 60 с                      |
| `mono_statement`      | GET /personal/statement/{account}/{from}/{to}  | токен       | 1 запит / 60 с, вікно до 31 дня     |
| `mono_set_webhook`    | POST /personal/webhook                         | токен       | URL має повертати HTTP 200 на GET   |

Усі грошові суми — цілі числа в мінімальних одиницях валюти
(копійках/центах).

### Збірка

Використовує [Task](https://taskfile.dev/) (`brew install go-task`):

```sh
task build     # bin/mono-go-mcp + bin/mono-go-cli
task install   # обидва у $GOPATH/bin
task check     # перевірка форматування + vet + збірка
task --list    # усі задачі
```

### CI / гейт злиття PR

Кожен PR проходить [`.github/workflows/ci.yml`](.github/workflows/ci.yml):
`go vet`, тести з покриттям та **govulncheck**. Злиття блокується, якщо
**покриття нижче 90%** або знайдено вразливості, які код викликає
(`MIN_COVERAGE` на початку workflow). Покриття публікується на Codecov,
кількість вразливостей — як shields.io бейдж.

Або звичайним go:

```sh
go build ./... && go vet ./...
go build -o mono-go-mcp ./cmd/mono-go-mcp
```

### Запуск (MCP-сервер через stdio)

Персональний токен можна отримати на <https://api.monobank.ua/>
(особистий кабінет). Публічні інструменти працюють без нього.

Можна покласти токен у файл `.env` (скопіюйте з `.env.example`) у
каталог, з якого запускається сервер:

```sh
# .env
MONO_TOKEN="<ваш токен>"
```

```sh
./mono-go-mcp
```

…або експортувати його в шеллі (має пріоритет над `.env`):

```sh
export MONO_TOKEN="<ваш токен>"
./mono-go-mcp
```

Сервер спілкується через MCP over stdio і призначений для запуску з
боку MCP-клієнта. Приклад універсальної конфігурації клієнта:

```json
{
  "mcpServers": {
    "mono-go-mcp": {
      "command": "/path/to/mono-go-mcp",
      "env": { "MONO_TOKEN": "<ваш токен>" }
    }
  }
}
```

Необов'язкова змінна: `MONO_BASE_URL` перевизначає базу API (для
тестів).

Під час запуску сервер автоматично завантажує `.env` зі свого робочого
каталогу (відсутність файлу — не помилка; змінні середовища оболонки
мають пріоритет над значеннями з `.env`).

### CLI (mono-go-cli)

Термінальний клієнт для того ж API monobank — MCP-клієнт не потрібен.
Без прапорців виводить курси валют, інформацію про банк і, якщо задано
`MONO_TOKEN`, дані клієнта та **дві таблиці виписок** для типового
рахунку:

- **минулий місяць** — з 1-го числа попереднього місяця до 1-го числа
  поточного;
- **поточний місяць** — з 1-го числа поточного місяця до зараз.

```sh
task smoke                       # або: go run ./cmd/mono-go-cli
mono-go-cli -stmt                # лише таблиці виписок
mono-go-cli -info -stmt          # дані клієнта + виписки
mono-go-cli -stmt -account <ID>  # конкретний рахунок/банка (типово "0")
mono-go-cli -rates               # лише таблиця курсів
mono-go-cli -sync                # лише публічний ключ + час сервера
mono-go-cli -webhook <URL>       # встановити webhook URL
mono-go-cli -no-wait             # помилка 429 замість очікування
mono-go-cli -version
```

Два вікна виписок — окремі виклики `/personal/statement`, тому другий
може впертися в ліміт 60 с; тоді CLI чекає
(`X-Auth-Interval-Expires`, не більше 60 с) і повторює запит. Прапорець
`-no-wait` вимикає це очікування.

### Перевірка

### Документація

- [`docs/api.md`](docs/api.md) — довідник API monobank (ендпоінти,
  схеми, ліміти запитів) з посиланнями на офіційну документацію.
- [`docs/api-spec.json`](docs/api-spec.json) — витягнута специфікація
  OpenAPI 3.0.3 (v250818), джерело істини для типів.
- [`AGENTS.md`](AGENTS.md) — структура репозиторію та інструкції для
  агентів.

### Структура

```
mono-go-mcp/
├── Taskfile.yml         # task runner (taskfile.dev): build, install, check
├── .goreleaser.yaml     # реліз-конфіг: обидва бінарники, linux/darwin amd64+arm64, windows amd64
├── .github/workflows/   # release.yml: goreleaser на тегах v*
├── cmd/mono-go-mcp/     # точка входу MCP-сервера: .env → клієнт → інструменти → stdio
├── cmd/mono-go-cli/     # термінальний клієнт: виклики monoapi → таблиці; прапорці вибору частин
├── docs/                # api.md + api-spec.json
└── internal/
    ├── monoapi/         # HTTP-клієнт monobank (без залежностей від MCP)
    └── tools/           # шар MCP-інструментів (без деталей HTTP)
```

### Примітки

- Персональний API monobank призначений лише для особистого
  користування; не використовуйте цей сервер із чужими токенами (для
  цього потрібен корпоративний API).
- Ніколи не комітьте `.env` і справжній токен.