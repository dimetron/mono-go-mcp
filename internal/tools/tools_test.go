package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dimetron/mono-go-mcp/internal/monoapi"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newTestEnv wires a monoapi Client pointing at a mock monobank API to
// an MCP server through the SDK's in-memory transport, and returns the
// connected client session. Every HTTP call the tools make lands on
// handler, so each test decides what the "bank" answers.
func newTestEnv(t *testing.T, handler http.HandlerFunc) *mcp.ClientSession {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := monoapi.NewClient("test-token", srv.URL)

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0"}, nil)
	Register(server, client)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go server.Run(context.Background(), serverTransport) //nolint:errcheck // in-memory test transport

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := mcpClient.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// callTool invokes the named tool with raw JSON arguments and fails the
// test on malformed arguments.
func callTool(t *testing.T, cs *mcp.ClientSession, name, args string) (*mcp.CallToolResult, error) {
	t.Helper()
	m := map[string]any{}
	if args != "" {
		if err := json.Unmarshal([]byte(args), &m); err != nil {
			t.Fatalf("bad args JSON %q: %v", args, err)
		}
	}
	return cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: m})
}

// resultText flattens the text content blocks of a CallToolResult.
func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	var parts []string
	for _, c := range res.Content {
		tc, ok := c.(*mcp.TextContent)
		if !ok {
			t.Fatalf("unexpected content type %T", c)
		}
		parts = append(parts, tc.Text)
	}
	return strings.Join(parts, "\n")
}

// writeTestJSON writes v as an application/json response body.
func writeTestJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func TestListTools(t *testing.T) {
	cs := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	want := []string{"mono_bank_sync", "mono_client_info", "mono_currency_rates", "mono_set_webhook", "mono_statement"}
	got := make([]string, 0, len(res.Tools))
	for _, tl := range res.Tools {
		got = append(got, tl.Name)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("tools = %v, want %v", got, want)
	}
}

func TestCurrencyRatesTool(t *testing.T) {
	var sawToken bool
	cs := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bank/currency" {
			sawToken = r.Header.Get("X-Token") == "test-token"
			writeTestJSON(w, []monoapi.CurrencyPair{
				{CurrencyCodeA: 840, CurrencyCodeB: 980, RateSell: 44.5, RateBuy: 45.1, Date: 1700000000},
			})
			return
		}
		http.NotFound(w, r)
	})
	res, err := callTool(t, cs, "mono_currency_rates", "")
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", resultText(t, res))
	}
	if !sawToken {
		t.Error("X-Token header not sent")
	}
	if !strings.Contains(resultText(t, res), `"currencyCodeA":840`) {
		t.Errorf("structured content missing rates: %s", resultText(t, res))
	}
}

func TestBankSyncTool(t *testing.T) {
	cs := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bank/sync" {
			writeTestJSON(w, monoapi.SyncInfo{ServerKeyId: "kid", ServerPubKey: "pub", ServerTimeMsec: 1700000000000})
			return
		}
		http.NotFound(w, r)
	})
	res, err := callTool(t, cs, "mono_bank_sync", "")
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), `"serverKeyId":"kid"`) {
		t.Errorf("unexpected content: %s", resultText(t, res))
	}
}

func TestClientInfoTool(t *testing.T) {
	cs := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/personal/client-info" {
			writeTestJSON(w, monoapi.ClientInfo{ClientID: "cli1", Name: "Test", Accounts: []monoapi.Account{{ID: "acc1", Balance: 1000, CurrencyCode: 980}}})
			return
		}
		http.NotFound(w, r)
	})
	res, err := callTool(t, cs, "mono_client_info", "")
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), `"clientId":"cli1"`) {
		t.Errorf("unexpected content: %s", resultText(t, res))
	}
}

func TestStatementToolDefaultAccountAndOpenEnded(t *testing.T) {
	var gotPath string
	cs := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		// Echo handler: the asserted path tells us account/from/to.
		writeTestJSON(w, monoapi.StatementItems{{ID: "tx1", Amount: -250, Time: 1700000000}})
	})
	// Explicit empty account must default to "0"; to=0 means now, so
	// from must be recent enough to stay within the 31-day max range.
	from := time.Now().Unix() - 3600
	res, err := callTool(t, cs, "mono_statement",
		fmt.Sprintf(`{"account":"","from":%d}`, from))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", resultText(t, res))
	}
	if !strings.HasPrefix(gotPath, fmt.Sprintf("/personal/statement/0/%d/", from)) {
		t.Errorf("path = %q, want default account 0 and from=100", gotPath)
	}
	if !strings.Contains(resultText(t, res), `"amount":-250`) {
		t.Errorf("unexpected content: %s", resultText(t, res))
	}
}

func TestStatementToolExplicitWindow(t *testing.T) {
	var gotPath string
	cs := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeTestJSON(w, monoapi.StatementItems{})
	})
	res, err := callTool(t, cs, "mono_statement", `{"account":"jar1","from":100,"to":200}`)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", resultText(t, res))
	}
	if gotPath != "/personal/statement/jar1/100/200" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestStatementToolUpstreamError(t *testing.T) {
	cs := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errorDescription":"bad token"}`))
	})
	res, err := callTool(t, cs, "mono_statement", `{"from":100,"to":200}`)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Errorf("want IsError=true, got content %s", resultText(t, res))
	}
}

func TestSetWebhookTool(t *testing.T) {
	var gotURL string
	cs := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/personal/webhook" {
			var body monoapi.SetWebHook
			json.NewDecoder(r.Body).Decode(&body)
			gotURL = body.WebHookURL
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})
	res, err := callTool(t, cs, "mono_set_webhook", `{"webHookUrl":"https://example.com/hook"}`)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", resultText(t, res))
	}
	if gotURL != "https://example.com/hook" {
		t.Errorf("webhook URL received = %q", gotURL)
	}
	if !strings.Contains(resultText(t, res), "webhook set") {
		t.Errorf("unexpected content: %s", resultText(t, res))
	}
}

func TestSetWebhookToolRequiresURL(t *testing.T) {
	cs := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must not be called when webHookUrl is empty")
	})
	// An explicit empty URL passes schema validation (the key is
	// present) and must be rejected by the tool itself.
	res, err := callTool(t, cs, "mono_set_webhook", `{"webHookUrl":""}`)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Error("want IsError=true for empty webHookUrl")
	}
	if !strings.Contains(resultText(t, res), "webHookUrl is required") {
		t.Errorf("unexpected content: %s", resultText(t, res))
	}
}

func TestSetWebhookToolUpstreamError(t *testing.T) {
	cs := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	res, err := callTool(t, cs, "mono_set_webhook", `{"webHookUrl":"https://example.com/h"}`)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Error("want IsError=true on upstream 403")
	}
}

func TestStatementToolEmptyItemsNotNull(t *testing.T) {
	// An empty (but non-nil) JSON array must come back as an empty
	// list, not null, in the structured output.
	cs := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("[]"))
	})
	res, err := callTool(t, cs, "mono_statement",
		fmt.Sprintf(`{"account":"0","from":%d,"to":%d}`, time.Now().Unix()-3600, time.Now().Unix()))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), `[]`) {
		t.Errorf("want empty JSON array in output, got %s", resultText(t, res))
	}
}
