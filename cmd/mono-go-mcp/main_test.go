package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	// Missing file is not an error.
	if err := loadDotEnv(filepath.Join(dir, "missing.env")); err != nil {
		t.Errorf("missing file: %v", err)
	}

	// A real file loads into the environment.
	if err := os.WriteFile(path, []byte("TEST_LOAD_DOTENV=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadDotEnv(path); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if v := os.Getenv("TEST_LOAD_DOTENV"); v != "1" {
		t.Errorf("TEST_LOAD_DOTENV=%q, want 1", v)
	}
	os.Unsetenv("TEST_LOAD_DOTENV")
}

func TestNewServerServesTools(t *testing.T) {
	// Mock monobank API so tool calls succeed end to end.
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/bank/currency":
			json.NewEncoder(w).Encode([]map[string]any{{"currencyCodeA": 840, "currencyCodeB": 980, "rateSell": 44.5, "date": 1700000000}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srvClose(t, api)
	t.Setenv("MONO_TOKEN", "tok")
	t.Setenv("MONO_BASE_URL", api.URL)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go newServer().Run(context.Background(), serverTransport) //nolint:errcheck // in-memory test transport

	client := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	cs, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer cs.Close()

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(res.Tools) != 5 {
		t.Errorf("tools = %d, want 5", len(res.Tools))
	}

	call, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "mono_currency_rates",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if call.IsError {
		t.Errorf("unexpected tool error: %v", call.Content)
	}
	var sawRate bool
	for _, c := range call.Content {
		if tc, ok := c.(*mcp.TextContent); ok && strings.Contains(tc.Text, "rateSell") {
			sawRate = true
		}
	}
	if !sawRate {
		t.Errorf("rates not in tool output: %v", call.Content)
	}
}

// srvClose stops srv during cleanup; split out to keep the test table flat.
func srvClose(t *testing.T, srv *httptest.Server) {
	t.Cleanup(srv.Close)
}

func TestRunWithInMemoryTransport(t *testing.T) {
	// run() with a transport override behaves like main() minus the
	// stdio plumbing: env loaded, server built, tools served.
	t.Setenv("MONO_TOKEN", "tok")
	t.Setenv("MONO_BASE_URL", "http://127.0.0.1:1") // unreachable on purpose

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	transportOverride = func() mcp.Transport { return serverTransport }
	t.Cleanup(func() { transportOverride = nil })

	errCh := make(chan error, 1)
	go func() { errCh <- run(context.Background(), nil) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	cs, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(res.Tools) != 5 {
		t.Errorf("tools = %d, want 5", len(res.Tools))
	}
	cs.Close()
	if err := <-errCh; err != nil {
		t.Errorf("run: %v", err)
	}
}

func TestTransportForDefault(t *testing.T) {
	// No override: stdio.
	if transportFor(nil) == nil {
		t.Error("default transport must be stdio, got nil")
	}
	// With override: the supplied transport wins.
	serverTransport, _ := mcp.NewInMemoryTransports()
	transportOverride = func() mcp.Transport { return serverTransport }
	t.Cleanup(func() { transportOverride = nil })
	if transportFor(nil) != mcp.Transport(serverTransport) {
		t.Error("override not honored")
	}
}

func TestRunLoadDotEnvWarning(t *testing.T) {
	// A malformed .env makes loadDotEnv fail; run() must only warn
	// and keep serving.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("this is not valid env content!!\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MONO_TOKEN", "")
	t.Setenv("MONO_BASE_URL", "")

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	transportOverride = func() mcp.Transport { return serverTransport }
	t.Cleanup(func() { transportOverride = nil })
	errCh := make(chan error, 1)
	go func() { errCh <- run(context.Background(), nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	cs, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	cs.Close()
	if err := <-errCh; err != nil {
		t.Errorf("run with broken .env: %v", err)
	}
}
