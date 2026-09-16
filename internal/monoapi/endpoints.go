package monoapi

import "context"

// This file maps one method per documented endpoint.

// GetCurrencyRates returns the base list of monobank currency rates.
// Cached upstream; refreshed no more than once per 5 minutes.
// GET /bank/currency — no auth required.
func (c *Client) GetCurrencyRates(ctx context.Context) ([]CurrencyPair, error) {
	var out []CurrencyPair
	if err := c.get(ctx, "/bank/currency", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetBankSync returns the bank's public key, its ID and server time.
// GET /bank/sync — no auth required.
func (c *Client) GetBankSync(ctx context.Context) (*SyncInfo, error) {
	var out SyncInfo
	if err := c.get(ctx, "/bank/sync", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetClientInfo returns client info with all accounts and jars.
// Results are cached for 65 s (API limit: once per 60 seconds).
// GET /personal/client-info — requires X-Token.
func (c *Client) GetClientInfo(ctx context.Context) (*ClientInfo, error) {
	if v, ok := c.cache.get("client-info"); ok {
		return v.(*ClientInfo), nil
	}
	var out ClientInfo
	if err := c.get(ctx, "/personal/client-info", &out); err != nil {
		return nil, err
	}
	c.cache.set("client-info", &out)
	return &out, nil
}

// SetWebHook registers the webhook URL that will receive statement
// events. Monobank validates it with a GET that must answer exactly
// HTTP 200.
// POST /personal/webhook — requires X-Token.
func (c *Client) SetWebHook(ctx context.Context, webHookURL string) error {
	return c.post(ctx, "/personal/webhook", SetWebHook{WebHookURL: webHookURL}, nil)
}

// GetStatement returns statement items for account between from and to
// (Unix seconds). account "0" means the default account. Max range:
// 2,682,000 seconds (31 days + 1 hour). Results are cached for 65 s
// (API limit: once per 60 seconds) — but only for closed windows:
// a "to" of now (0) is never cached, since new transactions may land.
// GET /personal/statement/{account}/{from}/{to} — requires X-Token.
func (c *Client) GetStatement(ctx context.Context, account string, from, to int64) (StatementItems, error) {
	if from < 0 {
		return nil, &APIError{Endpoint: "statement", Body: "from must be a Unix timestamp in seconds"}
	}
	openEnded := to <= 0
	if openEnded {
		// Per spec: missing "to" means current time.
		to = nowUnix()
	}
	if to-from > StatementMaxRangeSeconds {
		return nil, &APIError{
			Endpoint: "statement",
			Body:     "range too large: max 2682000 seconds (31 days + 1 hour)",
		}
	}
	key := "statement:" + account + ":" + itoa(from) + ":" + itoa(to)
	if !openEnded {
		if v, ok := c.cache.get(key); ok {
			return v.(StatementItems), nil
		}
	}
	var out StatementItems
	err := c.get(ctx, "/personal/statement/"+account+"/"+itoa(from)+"/"+itoa(to), &out)
	if err != nil {
		return nil, err
	}
	if !openEnded {
		c.cache.set(key, out)
	}
	return out, nil
}
