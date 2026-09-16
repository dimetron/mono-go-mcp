# mono-go-mcp

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
task build     # bin/mono-go-mcp
task install   # into $GOPATH/bin
task check     # fmt gate + vet + build
task --list    # all tasks
```

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

### Verify

```sh
task smoke
# or: go run ./cmd/smoke
```

Always lists tools and calls the public endpoints. With `MONO_TOKEN`
(in the environment or `.env`) it also calls `mono_client_info` and
`mono_statement` for the **last month period** (30 days back → now,
default account, capped at the API's 31-day window) and prints a
per-transaction summary. Mind the 60 s rate limit when re-running.

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
run: `task release:build` (cross-builds all 5 targets into `dist/`),
validate config with `task release:check`.

## Structure

```
mono-go-mcp/
├── Taskfile.yml         # task runner (taskfile.dev): build, install, check
├── .goreleaser.yaml     # release config: linux/darwin amd64+arm64, windows amd64
├── .github/workflows/   # release.yml: goreleaser on v* tags
├── cmd/mono-go-mcp/     # entrypoint: .env → client → tools → stdio
├── cmd/smoke/           # dev smoke test (in-memory MCP + real API)
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
task build     # bin/mono-go-mcp
task install   # у $GOPATH/bin
task check     # перевірка форматування + vet + збірка
task --list    # усі задачі
```

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

### Перевірка

```sh
task smoke
# або: go run ./cmd/smoke
```

Завжди показує список інструментів і викликає публічні ендпоінти. Якщо
задано `MONO_TOKEN` (у середовищі або `.env`), також викликає
`mono_client_info` і `mono_statement` за **період останнього місяця**
(30 днів назад → зараз, типовий рахунок, з урахуванням обмеження вікна
API у 31 день) і виводить підсумок по транзакціях. Пам'ятайте про ліміт
60 с під час повторних запусків.

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
├── cmd/mono-go-mcp/     # точка входу: .env → клієнт → інструменти → stdio
├── cmd/smoke/           # розробницький smoke-тест (in-memory MCP + реальний API)
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