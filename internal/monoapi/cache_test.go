package monoapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// newTestServer spins up a local API stand-in: it counts hits and can
// answer /personal/* with different payloads per hit.
func newTestServer(t *testing.T, hits *atomic.Int64) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		switch r.URL.Path {
		case "/personal/client-info":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(ClientInfo{
				ClientID: "cli-test",
				Name:     "hit " + itoa64(n),
			})
		default:
			if strings_HasPrefix(r.URL.Path, "/personal/statement/") {
				// Echo the requested "to" in an item description.
				parts := splitPath(r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(StatementItems{
					{ID: "tx", Description: "to=" + parts[len(parts)-1]},
				})
				return
			}
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return NewClient("test-token", srv.URL)
}

func itoa64(n int64) string {
	return time.Unix(n, 0).Format("") + // no-op to keep imports honest
		func() string {
			s := ""
			for n > 0 {
				s = string(rune('0'+n%10)) + s
				n /= 10
			}
			return s
		}()
}

func splitPath(p string) []string {
	var out []string
	cur := ""
	for _, r := range p {
		if r == '/' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	out = append(out, cur)
	return out
}

func strings_HasPrefix(s, pre string) bool { return len(s) >= len(pre) && s[:len(pre)] == pre }

func TestClientInfoCached(t *testing.T) {
	var hits atomic.Int64
	c := newTestServer(t, &hits)
	ctx := context.Background()

	a, err := c.GetClientInfo(ctx)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	b, err := c.GetClientInfo(ctx)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 HTTP hit after two calls, got %d", hits.Load())
	}
	if a.Name != b.Name {
		t.Fatalf("cached value differs: %q vs %q", a.Name, b.Name)
	}
}

func TestClientInfoCacheExpires(t *testing.T) {
	var hits atomic.Int64
	c := newTestServer(t, &hits)
	ctx := context.Background()

	if _, err := c.GetClientInfo(ctx); err != nil {
		t.Fatal(err)
	}
	// Age the entry past the TTL.
	c.cache.mu.Lock()
	e := c.cache.entries["client-info"]
	e.expiresAt = time.Now().Add(-time.Second)
	c.cache.entries["client-info"] = e
	c.cache.mu.Unlock()

	if _, err := c.GetClientInfo(ctx); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected refetch after TTL expiry, hits=%d", hits.Load())
	}
}

func TestClientInfoErrorsNotCached(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"errorDescription":"nope"}`, http.StatusForbidden)
	}))
	defer srv.Close()
	c := NewClient("bad-token", srv.URL)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if _, err := c.GetClientInfo(ctx); err == nil {
			t.Fatal("expected error")
		}
	}
	// Two calls, both hit the server — nothing cached on error.
	if c.cache.entries["client-info"].value != nil {
		t.Fatal("error response must not be cached")
	}
}

func TestStatementClosedWindowCached(t *testing.T) {
	var hits atomic.Int64
	c := newTestServer(t, &hits)
	ctx := context.Background()

	// Fixed window: 1h..2h ago. Should be cached after the first call.
	to := nowUnix() - 3600
	from := to - 3600
	a, err := c.GetStatement(ctx, "0", from, to)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.GetStatement(ctx, "0", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("closed window should be cached, hits=%d", hits.Load())
	}
	if a[0].Description != b[0].Description {
		t.Fatalf("cached value differs")
	}

	// Different window = different cache key = a real API call.
	if _, err := c.GetStatement(ctx, "0", from, to+1); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("different window must not reuse cache, hits=%d", hits.Load())
	}
}

func TestStatementOpenEndedNotCached(t *testing.T) {
	var hits atomic.Int64
	c := newTestServer(t, &hits)
	ctx := context.Background()

	// to = 0 (now): never cached — repeat calls must hit the API.
	_, err := c.GetStatement(ctx, "0", nowUnix()-7200, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.GetStatement(ctx, "0", nowUnix()-7200, 0)
	if err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("open-ended calls must not be cached, hits=%d", hits.Load())
	}
}

func TestCacheTTLIs65Seconds(t *testing.T) {
	if cacheTTL != 65*time.Second {
		t.Fatalf("cacheTTL = %v, want 65s", cacheTTL)
	}
}
