# Monobank open API — reference

Source of truth: <https://api.monobank.ua/docs/index.html> — spec
**v250818**, OpenAPI **3.0.3**, base URL `https://api.monobank.ua`.
A verbatim extracted copy is kept in [`api-spec.json`](api-spec.json)
(the docs page embeds the spec as `__redoc_state` in HTML;
`/docs/openapi.json` returns 403).

Tags in the spec:

- **Публічні дані** — public data, no authorization.
- **Клієнтські персональні дані** — requires a personal `X-Token`,
  obtained at <https://api.monobank.ua/> (personal cabinet →
  "Отримати токен для персонального використання").

General rules:

- The personal API may only be used by the client personally. Building
  libraries that clients use directly (no customer data through the
  developer's nodes) does not require the corporate API; centralizing
  other people's data does, and the bank can sanction misuse.
- Errors return `{"errorDescription": "..."}`; automate on the HTTP
  status code, not the text.
- The API is unavailable to clients under 16; children's account data
  is available from the parent account.

## Endpoints

### GET /bank/currency — currency rates (public)

Base list of monobank currency rates. Cached upstream; refreshed no
more than once per 5 minutes.

Response `200` — array of `CurrencyInfo`:

| field           | type    | notes                                   |
|-----------------|---------|-----------------------------------------|
| `currencyCodeA` | int32   | ISO 4217 numeric, e.g. 840 (USD)        |
| `currencyCodeB` | int32   | ISO 4217 numeric, e.g. 980 (UAH)        |
| `date`          | int64   | Unix seconds                            |
| `rateSell`      | float   | optional                                |
| `rateBuy`       | float   | optional                                |
| `rateCross`     | float   | optional                                |

A pair may have one or more of `rateSell`, `rateBuy`, `rateCross`.

```sh
curl -s https://api.monobank.ua/bank/currency
```

### GET /bank/sync — bank public key (public)

Returns the bank's public key, its identifier, and server time.

Response `200` — `SyncInfo`:

| field             | type   | notes                                              |
|-------------------|--------|----------------------------------------------------|
| `serverKeyId`     | string | hex identifier of the key                          |
| `serverPubKey`    | string | Base64, Secp256k1, uncompressed                    |
| `serverTimeMsec`  | int64  | current server time, Unix ms                       |

### GET /personal/client-info — client info (X-Token)

Client's personal data, all accounts (`accounts`) and jars (`jars`).
**Rate limit: once per 60 seconds.**

Header: `X-Token: <personal token>`.

Response `200` — `UserInfo`:

| field         | type     | notes                                        |
|---------------|----------|----------------------------------------------|
| `clientId`    | string   | matches `id` on send.monobank.ua             |
| `name`        | string   | client name                                  |
| `webHookUrl`  | string   | current webhook, if set                      |
| `permissions` | string   | one letter per permission, e.g. `psfj`       |
| `accounts`    | array    | Account objects (below)                      |
| `jars`        | array    | Jar objects (below)                          |

`Account`:

| field           | type     | notes                                              |
|-----------------|----------|----------------------------------------------------|
| `id`            | string   | account id (use in statement calls)                |
| `sendId`        | string   | id for send.monobank.ua/{sendId}                   |
| `balance`       | int64    | minimal currency units (kopiykas/cents)            |
| `creditLimit`   | int64    | credit limit, minimal units                        |
| `type`          | string   | `black` \| `white` \| `platinum` \| `iron` \| `fop` \| `yellow` \| `eAid` |
| `currencyCode`  | int32    | ISO 4217 numeric                                   |
| `cashbackType`  | string   | `None` \| `UAH` \| `Miles`                         |
| `maskedPan`     | []string | masked card numbers (several on premium cards)     |
| `iban`          | string   | account IBAN                                       |

`Jar` (banka):

| field           | type   | notes                                   |
|-----------------|--------|-----------------------------------------|
| `id`            | string | jar id (usable in statement calls)      |
| `sendId`        | string | id for send.monobank.ua/{sendId}        |
| `title`         | string | jar name                                |
| `description`   | string | jar description                         |
| `currencyCode`  | int32  | ISO 4217 numeric                        |
| `balance`       | int64  | minimal units                           |
| `goal`          | int64  | target amount, minimal units            |

### POST /personal/webhook — set webhook (X-Token)

Registers the URL that receives statement events for personal, FOP and
jar accounts. Behavior:

- Validation: monobank sends a **GET** to the URL; the server must
  answer with exactly HTTP **200** (no other code).
- Events: POST to the URL with body
  `{type:"StatementItem", data:{account:"...", statementItem:{…StatementItem}}}`.
- If the service does not answer within 5 s, monobank retries after
  60 s and 600 s; if the third attempt fails, the function is disabled.

Request body — `SetWebHook`:

| field        | type   | notes                            |
|--------------|--------|----------------------------------|
| `webHookUrl` | string | HTTPS URL that answers GET → 200 |

### GET /personal/statement/{account}/{from}/{to} — statement (X-Token)

Statement items for `account` (id from client-info, a jar id, or `0`
for the default account) from `from` to `to` Unix **seconds**.

- Max window: **2,682,000 seconds** (31 days + 1 hour).
- `to` omitted → current time.
- **Rate limit: once per 60 seconds.**

Response `200` — array of `StatementItem`:

| field             | type   | notes                                             |
|-------------------|--------|---------------------------------------------------|
| `id`              | string | unique transaction id                             |
| `time`            | int64  | Unix seconds                                      |
| `description`     | string | transaction description                           |
| `mcc`             | int32  | ISO 18245 MCC                                     |
| `originalMcc`     | int32  | original MCC                                      |
| `hold`            | bool   | authorization hold                                |
| `amount`          | int64  | account currency, minimal units                   |
| `operationAmount` | int64  | operation currency, minimal units                 |
| `currencyCode`    | int32  | ISO 4217 numeric                                  |
| `commissionRate`  | int64  | commission, minimal units                         |
| `cashbackAmount`  | int64  | cashback, minimal units                           |
| `balance`         | int64  | account balance after operation, minimal units    |
| `comment`         | string | user comment (may be absent)                      |
| `receiptId`       | string | check.gov.ua receipt (may be absent)              |
| `invoiceId`       | string | FOP receipt number (income operations)            |
| `counterEdrpou`   | string | counterparty EDRPOU (FOP statements only)         |
| `counterIban`     | string | counterparty IBAN (FOP statements only)           |
| `counterName`     | string | counterparty name                                 |

```sh
TOKEN=... # personal token
curl -s -H "X-Token: $TOKEN" \
  "https://api.monobank.ua/personal/statement/0/1554466347/1554552747"
```

## Errors and rate limits

- **401/403** — missing/invalid `X-Token`:
  `{"errorDescription":"Missing one of required headers 'X-Token'"}`.
- **429** — rate limit exceeded; the response carries
  `X-Auth-Interval-Expires` (seconds until the window resets).
- **404** — bad request data (e.g. invalid account or time range).
- On 429 the API may also answer `503` briefly during maintenance.

Rate limits summarized:

| endpoint                  | limit                     |
|---------------------------|---------------------------|
| GET /bank/currency        | upstream 5-min cache      |
| GET /personal/client-info | 1 per 60 s                |
| GET /personal/statement   | 1 per 60 s, 31-day window |
| POST /personal/webhook    | —                         |

**Local caching**: to stay under the 60 s limit, this server caches
`/personal/client-info` and `/personal/statement` responses in-process
for **65 s** (60 s + safety margin). Repeated tool calls within the
window reuse the cached payload. Statement caching applies only to
closed windows — a `to` of "now" is always fetched fresh so new
transactions appear. Failed responses are never cached.

## Webhook event payload

```json
{
  "type": "StatementItem",
  "data": {
    "account": "kKGVoZuHWzqVoZuH",
    "statementItem": { ...StatementItem... }
  }
}
```

The receiving endpoint must respond HTTP 200 within 5 s; retries come
after 60 s and 600 s.

## MCP tool mapping (this server)

| MCP tool              | endpoint                                       |
|-----------------------|------------------------------------------------|
| `mono_currency_rates` | GET /bank/currency                             |
| `mono_bank_sync`      | GET /bank/sync                                 |
| `mono_client_info`    | GET /personal/client-info                      |
| `mono_statement`      | GET /personal/statement/{account}/{from}/{to}  |
| `mono_set_webhook`    | POST /personal/webhook                         |

Community: <https://t.me/machine_learning_monobank> (API Telegram group
linked from the docs page).