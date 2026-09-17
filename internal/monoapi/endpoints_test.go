package monoapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

// newMockAPI builds a Client whose every HTTP call lands on handler.
// This is the endpoint-level mock used to exercise the client's URL
// construction, headers, error mapping and caching end to end.
func newMockAPI(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := newServer(t, handler)
	return NewClient("test-token", srv.URL)
}

// wantPath asserts method, full path and the X-Token header of one
// captured request.
func wantRequest(r *http.Request, method, path string) error {
	if r.Method != method {
		return errors.New("method " + r.Method + ", want " + method)
	}
	if r.URL.Path != path {
		return errors.New("path " + r.URL.Path + ", want " + path)
	}
	if got := r.Header.Get("X-Token"); got != "test-token" {
		return errors.New("X-Token " + got + ", want test-token")
	}
	return nil
}

func TestGetCurrencyRates(t *testing.T) {
	var gotPath error
	c := newMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = wantRequest(r, http.MethodGet, "/bank/currency")
		writeJSON(w, []CurrencyPair{
			{CurrencyCodeA: 840, CurrencyCodeB: 980, RateSell: 44.5, Date: 1700000000},
		})
	})
	pairs, err := c.GetCurrencyRates(context.Background())
	if err != nil {
		t.Fatalf("GetCurrencyRates: %v", err)
	}
	if gotPath != nil {
		t.Fatal(gotPath)
	}
	if len(pairs) != 1 || pairs[0].CurrencyCodeA != 840 {
		t.Fatalf("unexpected pairs: %+v", pairs)
	}
}

func TestGetBankSync(t *testing.T) {
	var gotPath error
	c := newMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = wantRequest(r, http.MethodGet, "/bank/sync")
		writeJSON(w, SyncInfo{ServerKeyId: "kid", ServerPubKey: "pub", ServerTimeMsec: 1700000000000})
	})
	si, err := c.GetBankSync(context.Background())
	if err != nil {
		t.Fatalf("GetBankSync: %v", err)
	}
	if gotPath != nil {
		t.Fatal(gotPath)
	}
	if si.ServerKeyId != "kid" || si.ServerPubKey != "pub" {
		t.Fatalf("unexpected sync info: %+v", si)
	}

	// Error path.
	c2 := newMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	if _, err := c2.GetBankSync(context.Background()); err == nil {
		t.Error("want error on 403, got nil")
	}
}

func TestSetWebHook(t *testing.T) {
	var gotPath error
	var gotBody SetWebHook
	c := newMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if err := wantRequest(r, http.MethodPost, "/personal/webhook"); err != nil {
			gotPath = err
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			gotPath = errors.New("Content-Type " + ct + ", want application/json")
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	})
	err := c.SetWebHook(context.Background(), "https://example.com/hook")
	if err != nil {
		t.Fatalf("SetWebHook: %v", err)
	}
	if gotPath != nil {
		t.Fatal(gotPath)
	}
	if gotBody.WebHookURL != "https://example.com/hook" {
		t.Fatalf("webhook body = %q", gotBody.WebHookURL)
	}
}

func TestGetClientInfo(t *testing.T) {
	var gotPath error
	c := newMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = wantRequest(r, http.MethodGet, "/personal/client-info")
		writeJSON(w, ClientInfo{ClientID: "cli", Name: "Test", Accounts: []Account{{
			ID: "acc1", Balance: 12345, CurrencyCode: 980, Type: "black",
		}}})
	})
	ci, err := c.GetClientInfo(context.Background())
	if err != nil {
		t.Fatalf("GetClientInfo: %v", err)
	}
	if gotPath != nil {
		t.Fatal(gotPath)
	}
	if ci.ClientID != "cli" || len(ci.Accounts) != 1 || ci.Accounts[0].ID != "acc1" {
		t.Fatalf("unexpected client info: %+v", ci)
	}
}

func TestGetStatement(t *testing.T) {
	var gotPath error
	var gotPathStr string
	c := newMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		gotPathStr = r.URL.Path
		if r.URL.Path != "/personal/statement/acc1/100/200" {
			gotPath = errors.New("unexpected statement path " + r.URL.Path)
		}
		writeJSON(w, StatementItems{{ID: "tx1", Amount: -500, Time: 150}})
	})
	items, err := c.GetStatement(context.Background(), "acc1", 100, 200)
	if err != nil {
		t.Fatalf("GetStatement: %v", err)
	}
	_ = gotPathStr
	if gotPath != nil {
		t.Fatal(gotPath)
	}
	if len(items) != 1 || items[0].ID != "tx1" || items[0].Amount != -500 {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestGetStatementValidation(t *testing.T) {
	c := newMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must not be called for invalid ranges")
	})
	ctx := context.Background()

	// Negative from.
	if _, err := c.GetStatement(ctx, "0", -1, 100); !isStatementUsageErr(err) {
		t.Errorf("negative from: want usage APIError, got %v", err)
	}
	// Range beyond the documented maximum.
	if _, err := c.GetStatement(ctx, "0", 0, StatementMaxRangeSeconds+1); !isStatementUsageErr(err) {
		t.Errorf("range too large: want usage APIError, got %v", err)
	}
}

func TestGetStatementUpstreamError(t *testing.T) {
	c := newMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errorDescription":"no"}`))
	})
	_, err := c.GetStatement(context.Background(), "0", 100, 200)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 APIError, got %v", err)
	}
}

func isStatementUsageErr(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Endpoint == "statement" && apiErr.StatusCode == 0
}

func TestAPITimeout(t *testing.T) {
	// A server that never answers: the client must surface the error
	// instead of hanging for the 30 s HTTP timeout.
	done := make(chan struct{})
	c := newMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		close(done)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer func() { <-done }() // let the handler finish before srv.Close
	defer cancel()
	if _, err := c.GetCurrencyRates(ctx); err == nil {
		t.Error("want error on expired context, got nil")
	}
}

func TestAPIError429(t *testing.T) {
	c := newMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Auth-Interval-Expires", "37")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"errorDescription":"too many requests"}`))
	})
	_, err := c.GetClientInfo(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %v", err)
	}
	if !apiErr.IsRateLimited() {
		t.Errorf("IsRateLimited()=false for %v", apiErr)
	}
	if apiErr.RateLimitResetSec != 37 {
		t.Errorf("RateLimitResetSec=%d, want 37", apiErr.RateLimitResetSec)
	}
	want := "monobank API /personal/client-info: {\"errorDescription\":\"too many requests\"} (rate limit resets in 37s)"
	if apiErr.Error() != want {
		t.Errorf("Error()=%q, want %q", apiErr.Error(), want)
	}
}

func TestAPIErrorNoBody(t *testing.T) {
	c := newMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	_, err := c.GetClientInfo(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %v", err)
	}
	if apiErr.Error() != "monobank API /personal/client-info: 403 Forbidden" {
		t.Errorf("empty-body error text %q", apiErr.Error())
	}
}

func TestNewClientDefaults(t *testing.T) {
	// Empty base URL falls back to BaseURL, trailing slashes trimmed.
	c := NewClient("t", "https://example.com///")
	if c.baseURL != "https://example.com" {
		t.Errorf("baseURL=%q", c.baseURL)
	}
	c = NewClient("t", "")
	if c.baseURL != BaseURL {
		t.Errorf("default baseURL=%q, want %q", c.baseURL, BaseURL)
	}
}

func TestDecodeError(t *testing.T) {
	c := newMockAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	})
	if _, err := c.GetCurrencyRates(context.Background()); err == nil {
		t.Error("want decode error, got nil")
	}
}

func TestResetSeconds(t *testing.T) {
	for in, want := range map[string]int{"": 0, "7": 7, "42": 42, "12x": 0, "-3": 0, "003": 3, "007": 7} {
		if got := resetSeconds(in); got != want {
			t.Errorf("resetSeconds(%q)=%d, want %d", in, got, want)
		}
	}
}

func TestDoInvalidURL(t *testing.T) {
	// A control character in the base URL makes http.NewRequest fail;
	// do must surface that error instead of panicking.
	c := NewClient("t", "http://bad\x7f.example.com")
	if err := c.get(context.Background(), "/bank/currency", new(any)); err == nil {
		t.Error("want error for invalid URL, got nil")
	}
}
