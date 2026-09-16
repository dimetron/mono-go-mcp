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
	if err := loadDotEnv(".env"); err != nil {
		log.Printf("warning: %v", err)
	}

	client := monoapi.NewClient(
		os.Getenv("MONO_TOKEN"),
		os.Getenv("MONO_BASE_URL"), // optional override, e.g. for tests
	)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mono-go-mcp",
		Version: version,
	}, nil)

	tools.Register(server, client)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

// loadDotEnv loads key=value pairs from path into the environment,
// skipping keys that are already set. A missing file is not an error.
func loadDotEnv(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return godotenv.Load(path)
}
