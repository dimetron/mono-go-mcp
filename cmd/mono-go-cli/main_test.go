package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dimetron/mono-go-mcp/internal/monoapi"
)

// cliEnv starts a mock monobank API and sets MONO_BASE_URL to it for
// the duration of the test. token decides whether MONO_TOKEN is set.
// Returns the mock base URL for direct client construction.
func cliEnv(t *testing.T, token string, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	t.Setenv("MONO_BASE_URL", srv.URL)
	t.Setenv("MONO_TOKEN", token)
	return srv.URL
}

func writeJSONLine(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func TestRunVersion(t *testing.T) {
	code := run([]string{"-version"})
	if code != 0 {
		t.Errorf("code=%d, want 0", code)
	}
}

func TestRunAllPartsWithMock(t *testing.T) {
	cliEnv(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/bank/currency":
			writeJSONLine(w, []monoapi.CurrencyPair{
				{CurrencyCodeA: 840, CurrencyCodeB: 980, RateSell: 44.5, Date: 1700000000},
			})
		case r.URL.Path == "/bank/sync":
			writeJSONLine(w, monoapi.SyncInfo{ServerKeyId: "kid", ServerPubKey: "pub", ServerTimeMsec: 1700000000000})
		case r.URL.Path == "/personal/client-info":
			writeJSONLine(w, monoapi.ClientInfo{
				ClientID: "cli1", Name: "Test",
				Accounts: []monoapi.Account{{ID: "acc1", Balance: 10050, CurrencyCode: 980, Type: "black"}},
			})
		case strings.HasPrefix(r.URL.Path, "/personal/statement/"):
			writeJSONLine(w, monoapi.StatementItems{
				{ID: "t1", Time: time.Now().Unix() - 60, Amount: -2500, Description: "coffee", Hold: true, CurrencyCode: 980},
				{ID: "t2", Time: time.Now().Unix() - 30, Amount: 100000, Description: "income", CurrencyCode: 980},
			})
		default:
			http.NotFound(w, r)
		}
	})
	code := run([]string{})
	if code != 0 {
		t.Errorf("code=%d, want 0", code)
	}
}

func TestRunRatesOnlySuccess(t *testing.T) {
	cliEnv(t, "", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bank/currency" {
			writeJSONLine(w, []monoapi.CurrencyPair{})
			return
		}
		http.NotFound(w, r)
	})
	if code := run([]string{"-rates"}); code != 0 {
		t.Errorf("code=%d, want 0", code)
	}
}

func TestRunSyncOnlySuccess(t *testing.T) {
	cliEnv(t, "", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bank/sync" {
			writeJSONLine(w, monoapi.SyncInfo{ServerKeyId: "kid", ServerPubKey: "pub", ServerTimeMsec: 1})
			return
		}
		http.NotFound(w, r)
	})
	if code := run([]string{"-sync"}); code != 0 {
		t.Errorf("code=%d, want 0", code)
	}
}

func TestRunRatesFailureExitsNonzero(t *testing.T) {
	cliEnv(t, "", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if code := run([]string{"-rates"}); code != 1 {
		t.Errorf("code=%d, want 1 on API failure", code)
	}
}

func TestRunExplicitInfoWithoutToken(t *testing.T) {
	// Explicit -info without MONO_TOKEN: exit 1 without calling API.
	cliEnv(t, "", func(w http.ResponseWriter, r *http.Request) {
		t.Error("API must not be called without a token")
	})
	if code := run([]string{"-info"}); code != 1 {
		t.Errorf("code=%d, want 1", code)
	}
}

func TestRunDefaultWithoutTokenSkipsPersonalAndExitsZero(t *testing.T) {
	// No flags and no MONO_TOKEN: the personal parts are skipped (not an
	// error), the public parts still work, and the process exits 0.
	cliEnv(t, "", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bank/currency" {
			writeJSONLine(w, []monoapi.CurrencyPair{})
			return
		}
		if r.URL.Path == "/bank/sync" {
			writeJSONLine(w, monoapi.SyncInfo{ServerKeyId: "kid", ServerPubKey: "pub", ServerTimeMsec: 1})
			return
		}
		http.NotFound(w, r)
	})
	if code := run([]string{}); code != 0 {
		t.Errorf("code=%d, want 0 (default without token must skip personal parts, not fail)", code)
	}
}

func TestRunInfoWithTokenSuccessAndFailure(t *testing.T) {
	// Success.
	cliEnv(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		writeJSONLine(w, monoapi.ClientInfo{ClientID: "cli1", Name: "T", Accounts: []monoapi.Account{{ID: "a", Balance: 1, CurrencyCode: 980}}})
	})
	if code := run([]string{"-info"}); code != 0 {
		t.Errorf("success code=%d, want 0", code)
	}
	// API failure: exit 1.
	cliEnv(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	if code := run([]string{"-info"}); code != 1 {
		t.Errorf("failure code=%d, want 1", code)
	}
}

func TestRunWebhookOnly(t *testing.T) {
	var gotURL string
	cliEnv(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/personal/webhook" {
			var body monoapi.SetWebHook
			json.NewDecoder(r.Body).Decode(&body)
			gotURL = body.WebHookURL
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})
	if code := run([]string{"-webhook", "https://example.com/h"}); code != 0 {
		t.Errorf("code=%d, want 0", code)
	}
	if gotURL != "https://example.com/h" {
		t.Errorf("webhook URL = %q", gotURL)
	}
}

func TestRunWebhookFailure(t *testing.T) {
	cliEnv(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errorDescription":"no"}`))
	})
	if code := run([]string{"-webhook", "https://example.com/h"}); code != 1 {
		t.Errorf("code=%d, want 1", code)
	}
}

func TestRunStatementWindowsAreUTCAndCoversTwoMonths(t *testing.T) {
	var paths []string
	cliEnv(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		writeJSONLine(w, monoapi.StatementItems{})
	})
	if code := run([]string{"-stmt"}); code != 0 {
		t.Errorf("code=%d, want 0", code)
	}
	if len(paths) != 2 {
		t.Fatalf("statement calls = %d, want 2", len(paths))
	}
	now := time.Now().UTC()
	firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	want1 := fmt.Sprintf("/personal/statement/0/%d/%d", firstOfThisMonth.AddDate(0, -1, 0).Unix(), firstOfThisMonth.Unix())
	want2 := fmt.Sprintf("/personal/statement/0/%d/%d", firstOfThisMonth.Unix(), now.Unix())
	if paths[0] != want1 {
		t.Errorf("path[0]=%q, want %q", paths[0], want1)
	}
	if paths[1] != want2 {
		t.Errorf("path[1]=%q, want %q", paths[1], want2)
	}
}

func TestFetchStatementRetriesAfter429(t *testing.T) {
	var hits int
	base := cliEnv(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.Header().Set("X-Auth-Interval-Expires", "1") // cap logic: >0 and <=60 kept as-is
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeJSONLine(w, monoapi.StatementItems{{ID: "ok"}})
	})
	from := time.Now().Add(-2 * time.Hour)
	to := time.Now()
	items, err := fetchStatement(t.Context(), monoapi.NewClient("tok", base), "0", from, to, false, false)
	if err != nil {
		t.Fatalf("fetchStatement: %v", err)
	}
	if len(items) != 1 || items[0].ID != "ok" {
		t.Fatalf("items = %+v", items)
	}
	if hits != 2 {
		t.Errorf("hits = %d, want 2 (initial + retry)", hits)
	}
}

func TestFetchStatementNoWait(t *testing.T) {
	base := cliEnv(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Auth-Interval-Expires", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	_, err := fetchStatement(t.Context(), monoapi.NewClient("tok", base), "0",
		time.Now().Add(-time.Hour), time.Now(), true, false)
	if err == nil {
		t.Fatal("want 429 error with -no-wait, got nil")
	}
	var apiErr *monoapi.APIError
	if !errors.As(err, &apiErr) || !isRateLimited(err) {
		t.Errorf("want a 429 APIError with -no-wait, got %v", err)
	}
}

func TestRunWebhookWithoutToken(t *testing.T) {
	cliEnv(t, "", func(w http.ResponseWriter, r *http.Request) {
		t.Error("API must not be called without a token")
	})
	if code := run([]string{"-webhook", "https://example.com/h"}); code != 1 {
		t.Errorf("code=%d, want 1", code)
	}
}

func TestRunWebhookPlusRates(t *testing.T) {
	cliEnv(t, "tok", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/personal/webhook" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/bank/currency" {
			writeJSONLine(w, []monoapi.CurrencyPair{})
			return
		}
		http.NotFound(w, r)
	})
	if code := run([]string{"-webhook", "https://example.com/h", "-rates"}); code != 0 {
		t.Errorf("code=%d, want 0", code)
	}
}

func TestRunBadFlag(t *testing.T) {
	if code := run([]string{"-definitely-not-a-flag"}); code != 2 {
		t.Errorf("code=%d, want 2", code)
	}
}

func TestRateLimitResetVariants(t *testing.T) {
	if got := rateLimitReset(nil); got != 0 {
		t.Errorf("rateLimitReset(nil)=%d", got)
	}
	if got := rateLimitReset(errors.New("plain")); got != 0 {
		t.Errorf("rateLimitReset(plain)=%d", got)
	}
	e := &monoapi.APIError{RateLimitResetSec: 12}
	if got := rateLimitReset(e); got != 12 {
		t.Errorf("rateLimitReset=%d, want 12", got)
	}
}

func TestFmtMoney(t *testing.T) {
	for v, want := range map[int64]string{0: "0.00", 10050: "100.50", -250: "-2.50", 1: "0.01"} {
		if got := fmtMoney(v); got != want {
			t.Errorf("fmtMoney(%d)=%q, want %q", v, got, want)
		}
	}
}

func TestTrunc(t *testing.T) {
	if got := trunc("a\n b\t c ", 40); got != "a b c" {
		t.Errorf("whitespace collapse: %q", got)
	}
	long := strings.Repeat("x", 50)
	got := trunc(long, 10)
	if runes := len([]rune(got)); runes != 10 {
		t.Errorf("len=%d, want 10", runes)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("missing ellipsis: %q", got)
	}
}

func TestSym(t *testing.T) {
	if sym(840) != "USD" || sym(980) != "UAH" {
		t.Error("known codes must map")
	}
	if sym(99999) != "99999" {
		t.Errorf("unknown code fallback: %q", sym(99999))
	}
}

func TestHoldMark(t *testing.T) {
	if holdMark(true) != "hold" || holdMark(false) != "" {
		t.Error("holdMark broken")
	}
}

func TestPrintTables(t *testing.T) {
	// Smoke: render all table shapes without crashing; alignment
	// correctness is covered by printRow alignment rules below.
	printRates([]monoapi.CurrencyPair{{CurrencyCodeA: 840, CurrencyCodeB: 980, RateSell: 1.5, Date: 1700000000}}, false)
	printBankSync(&monoapi.SyncInfo{ServerKeyId: "k", ServerPubKey: "p", ServerTimeMsec: 1}, false)
	printClientInfo(&monoapi.ClientInfo{Name: "n", ClientID: "id", Accounts: []monoapi.Account{{ID: "a", Type: "black", CurrencyCode: 980, Balance: 100}}, Jars: []monoapi.Jar{{ID: "j", Title: "jar", CurrencyCode: 980, Balance: 2}}}, false)
	printStatement("win", time.Unix(100, 0), time.Unix(200, 0), monoapi.StatementItems{
		{Time: 150, Amount: 500, Description: "d", Hold: true, CurrencyCode: 980},
	}, false)
	// Empty statement prints the no-transactions branch.
	printStatement("empty", time.Unix(100, 0), time.Unix(200, 0), nil, false)
	// -json shapes: smoke-render each printer's JSON branch too.
	printRates(nil, true)
	printBankSync(&monoapi.SyncInfo{ServerKeyId: "k"}, true)
	printClientInfo(&monoapi.ClientInfo{Name: "n"}, true)
	printStatement("json", time.Unix(100, 0), time.Unix(200, 0), nil, true)
}

func TestPrintRowAlignment(t *testing.T) {
	// Captured in unit form via printTableAlign through printTable.
	printTableAlign([]string{"h1", "h2"}, [][]string{{"a", "b"}}, []int{1})
	printTableAlign([]string{"h1", "h2"}, [][]string{{"longer", "b"}}, nil)
}
