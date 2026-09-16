// Package tools wires monobank API endpoints into MCP tools.
//
// Handlers use the SDK's ToolHandlerFor[In, Out]: the In type becomes
// the input schema (via jsonschema tags), the Out value is returned as
// StructuredContent + JSON text content, and a returned error is
// automatically packed into a CallToolResult with IsError=true.
package tools

import (
	"context"
	"fmt"

	"github.com/dimetron/mono-go-mcp/internal/monoapi"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register adds all monobank tools to the server.
func Register(server *mcp.Server, client *monoapi.Client) {
	// Public endpoints (no token needed).
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mono_currency_rates",
		Description: "Get monobank currency rates (public, no token). Cached upstream, refreshed every 5 minutes.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []monoapi.CurrencyPair, error) {
		v, err := client.GetCurrencyRates(ctx)
		return nil, v, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mono_bank_sync",
		Description: "Get the bank's public key (Secp256k1) and server time (public, no token).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, *monoapi.SyncInfo, error) {
		v, err := client.GetBankSync(ctx)
		return nil, v, err
	})

	// Personal endpoints (require MONO_TOKEN).
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mono_client_info",
		Description: "Get client info: personal data, all accounts and jars. Rate limit: 1 request per 60 seconds. Requires MONO_TOKEN.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, *monoapi.ClientInfo, error) {
		v, err := client.GetClientInfo(ctx)
		return nil, v, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mono_statement",
		Description: "Get statement (transactions) for an account between two Unix timestamps (seconds). account \"0\" = default account; max range 2682000s (31 days + 1h). Rate limit: 1 request per 60 seconds. Requires MONO_TOKEN.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in statementInput) (*mcp.CallToolResult, monoapi.StatementItems, error) {
		if in.Account == "" {
			in.Account = "0"
		}
		items, err := client.GetStatement(ctx, in.Account, in.From, in.To)
		if items == nil {
			items = monoapi.StatementItems{}
		}
		return nil, items, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mono_set_webhook",
		Description: "Set the webhook URL that receives statement events. Monobank validates the URL with a GET that must answer exactly HTTP 200. Requires MONO_TOKEN.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in setWebhookInput) (*mcp.CallToolResult, map[string]string, error) {
		if in.WebHookURL == "" {
			return nil, nil, fmt.Errorf("webHookUrl is required")
		}
		if err := client.SetWebHook(ctx, in.WebHookURL); err != nil {
			return nil, nil, err
		}
		return nil, map[string]string{"status": "webhook set", "webHookUrl": in.WebHookURL}, nil
	})
}

type statementInput struct {
	Account string `json:"account" jsonschema:"account ID from mono_client_info, \"0\" for the default account, or a jar ID"`
	From    int64  `json:"from" jsonschema:"statement start time, Unix seconds"`
	To      int64  `json:"to,omitempty" jsonschema:"statement end time, Unix seconds; omitted or 0 means now"`
}

type setWebhookInput struct {
	WebHookURL string `json:"webHookUrl" jsonschema:"HTTPS URL that must respond with exactly HTTP 200 to a validation GET"`
}
