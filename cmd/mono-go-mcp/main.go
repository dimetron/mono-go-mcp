// Command mono-go-mcp is an MCP server exposing the monobank open API
// (https://api.monobank.ua/docs/index.html) as tools over stdio.
//
// Set MONO_TOKEN to a personal token from https://api.monobank.ua/
// for the /personal/* tools; public tools work without it.
//
// Environment is loaded from the nearest .env file (project root or
// CWD) at startup; variables already set in the environment win.
package main

import (
	"context"
	"log"
	"os"

	"github.com/dimetron/mono-go-mcp/internal/monoapi"
	"github.com/dimetron/mono-go-mcp/internal/tools"
	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const serverVersion = "0.1.0"

// version is set at build time via -ldflags "-X main.version=...".
var version = serverVersion

func main() {
	log.SetFlags(0) // plain log lines; MCP stderr stays readable
	if err := run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

// run loads the .env, builds the server and serves MCP over the
// transport chosen by args ("stdio" or absent: stdio). Split out of
// main so tests can exercise it with an in-memory transport instead.
func run(ctx context.Context, args []string) error {
	if err := loadDotEnv(".env"); err != nil {
		log.Printf("warning: %v", err)
	}
	return newServer().Run(ctx, transportFor(args))
}

// transportFor picks the MCP transport: stdio by default. transportHook
// is nil in production; tests set it to supply an in-memory transport.
var transportOverride func() mcp.Transport

func transportFor(args []string) mcp.Transport {
	if transportOverride != nil {
		return transportOverride()
	}
	return &mcp.StdioTransport{}
}

// newServer builds the MCP server with all monobank tools wired to a
// monoapi client configured from the environment. Split out of main so
// tests can connect an in-memory transport to it.
func newServer() *mcp.Server {
	client := monoapi.NewClient(
		os.Getenv("MONO_TOKEN"),
		os.Getenv("MONO_BASE_URL"), // optional override, e.g. for tests
	)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mono-go-mcp",
		Version: version,
	}, nil)

	tools.Register(server, client)
	return server
}

// loadDotEnv loads key=value pairs from path into the environment,
// skipping keys that are already set. A missing file is not an error.
func loadDotEnv(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return godotenv.Load(path)
}
