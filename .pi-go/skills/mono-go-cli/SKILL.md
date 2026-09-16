---
name: mono-go-cli
description: Query monobank (Ukrainian bank) from the terminal — currency rates, client info, and account statements via the mono-go-cli binary in this repo. Use whenever the user mentions monobank, "my balance", "my statements", "курси валют", "виписка", or asks to check card/accounts activity.
tools: [Bash]
---

# Using mono-go-cli

Terminal client for the monobank open API. Installed in `$HOME/go/bin`
(`go env GOPATH`/bin) via `task install` — run it by bare name. Reads
`MONO_TOKEN` from the shell or the repo's `.env` (auto-loaded — run from
the repo root, or the token silently won't be found).

```sh
mono-go-cli                       # everything: rates + sync + (with token) client info + 2 statement tables
mono-go-cli -rates                # currency rates table only (public, no token)
mono-go-cli -sync                 # bank public key + server time only (public, no token)
mono-go-cli -info                 # client info: names, accounts, cards, jars
mono-go-cli -stmt                 # two statement tables for the default account:
                                  #   1st of last month → 1st of this month, and 1st of this month → now
mono-go-cli -stmt -account <ID>   # specific account/jar (ID from -info; "0" = default)
mono-go-cli -webhook <URL>        # set the statement webhook URL (POST /personal/webhook)
mono-go-cli -no-wait              # fail on 429 instead of waiting out the rate limit
mono-go-cli -version
```

If the binary is missing, reinstall it: `task install` (puts both
binaries into `$(go env GOPATH)/bin`).

## Golden rules for agent behavior

- Run from the repo root (`/Users/dimetron/p6s/pi-dev/mcp/mono-go-mcp`),
  otherwise `.env` won't be found and personal endpoints are skipped.
- **Money in CLI output is already major units** (hryvnias, e.g.
  `-3640.00`) — the binary converts from raw API integers itself. Never
  divide displayed amounts again. Raw integer kopiykas appear only in
  the MCP tools' JSON output (`mono_statement`, `mono_client_info`);
  divide those by 100 for UAH.
- `/personal/*` endpoints (`-info`, `-stmt`) are rate-limited to **1 per
  60 s**. The two statement windows are separate calls; the CLI waits
  (`X-Auth-Interval-Expires`, capped at 60 s) and retries once. Don't
  spam `-stmt` back-to-back; if you get 429, tell the user to wait.
- Statement data returned is a fixed two-month window; the CLI has no
  arbitrary date-range flags — custom ranges need the MCP server
  (`mono_statement` tool) instead.
- Token comes from the environment or the repo `.env` (git-ignored —
  never print its contents or commit it).
- A personal monobank token is personal use only; never point the CLI
  at anyone else's token.