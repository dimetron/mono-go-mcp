package main

// Smoke test: connect an in-process MCP client to the server and
// exercise tool listing plus API calls over the real monobank API.
//
// Public endpoints always run. If MONO_TOKEN (or .env) provides a
// token, the personal endpoints run too: mono_client_info followed by
// mono_statement for the last month period. Mind the 60-second
// rate limit on /personal/* when re-running.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/dimetron/mono-go-mcp/internal/monoapi"
	"github.com/dimetron/mono-go-mcp/internal/tools"
	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	ctx := context.Background()

	_ = godotenv.Load(".env")
	token := os.Getenv("MONO_TOKEN")

	server := mcp.NewServer(&mcp.Implementation{Name: "mono-go-mcp", Version: "test"}, nil)
	tools.Register(server, monoapi.NewClient(token, ""))

	// In-memory transport pair: server side runs inline.
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		log.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "smoke-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		log.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	list, err := session.ListTools(ctx, nil)
	if err != nil {
		log.Fatalf("ListTools: %v", err)
	}
	fmt.Println("tools:")
	for _, t := range list.Tools {
		fmt.Printf("  - %s\n", t.Name)
	}

	for _, name := range []string{"mono_currency_rates", "mono_bank_sync"} {
		out := callTool(ctx, session, name, nil)
		if name == "mono_currency_rates" {
			printRates(name, out)
			continue
		}
		printResult(name, out)
	}

	// Personal endpoints: only with a token.
	if token == "" {
		fmt.Println("\npersonal tools: skipped (no MONO_TOKEN)")
		fmt.Println("\nSMOKE_OK")
		return
	}

	info := callTool(ctx, session, "mono_client_info", nil)
	printResult("mono_client_info", info)
	if info.IsError {
		log.Fatal("mono_client_info failed; cannot continue with statement")
	}

	// Last month period: 30 days back up to now, default account "0".
	now := time.Now()
	from := now.AddDate(0, 0, -30)
	// Round down to whole seconds and cap at the API's max window
	// (2,682,000 s) to stay valid even if the window shifts.
	fromUnix := from.Unix()
	toUnix := now.Unix()
	if toUnix-fromUnix > monoapi.StatementMaxRangeSeconds {
		fromUnix = toUnix - monoapi.StatementMaxRangeSeconds
	}

	stmt := callTool(ctx, session, "mono_statement", map[string]any{
		"account": "0",
		"from":    fromUnix,
		"to":      toUnix,
	})
	printStatementSummary(stmt, fromUnix, toUnix)

	fmt.Println("\nSMOKE_OK")
}

// callTool invokes a tool and fails hard on transport-level errors.
func callTool(ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	out, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		log.Fatalf("CallTool %s: %v", name, err)
	}
	return out
}

// printResult prints a compact prefix of structured content.
func printResult(name string, out *mcp.CallToolResult) {
	b, _ := json.Marshal(out.StructuredContent)
	s := string(b)
	if s == "null" {
		s = "<no structured content>"
	}
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	status := "ok"
	if out.IsError {
		status = "tool error"
	}
	fmt.Printf("\n%s [%s] -> %s\n", name, status, s)
}

// currencySymbols maps ISO 4217 numeric codes to alphabetic symbols
// (subset covering monobank pairs; unknown codes print as numbers).
var currencySymbols = map[int]string{
	8: "ALL", 12: "DZD", 32: "ARS", 36: "AUD", 48: "BHD",
	50: "BDT", 51: "AMD", 68: "BOB", 96: "BND", 108: "BIF",
	124: "CAD", 152: "CLP", 156: "CNY", 170: "COP", 188: "CRC",
	192: "CUP", 203: "CZK", 208: "DKK", 230: "ETB", 262: "DJF",
	270: "GMD", 344: "HKD", 348: "HUF", 392: "JPY", 398: "KZT",
	504: "MAD", 512: "OMR", 578: "NOK", 608: "PHP", 634: "QAR",
	642: "RON", 643: "RUB", 646: "RWF", 682: "SAR", 702: "SGD",
	756: "CHF", 784: "AED", 818: "EGP", 826: "GBP", 840: "USD",
	906: "KWD", 933: "BYN", 936: "GHS", 941: "RSD", 944: "AZN",
	949: "TRY", 971: "AFN", 973: "AOA", 975: "BGN", 976: "CDF",
	978: "EUR", 980: "UAH", 981: "GEL", 985: "PLN", 986: "BRL",
}

// rateCell returns the printable cell for a rate: fixed precision or
// blank for the optional/absent cross rate.
func rateCell(r float64) string {
	if r == 0 {
		return ""
	}
	return fmt.Sprintf("%.4f", r)
}

// printRates renders mono_currency_rates output as a text table.
func printRates(name string, out *mcp.CallToolResult) {
	if out.IsError {
		// Fall back to the generic printer for error results.
		printResult(name, out)
		return
	}
	fmt.Printf("\n%s [%s]\n", name, "ok")
	b, err := json.Marshal(out.StructuredContent)
	if err != nil {
		log.Fatalf("marshal rates: %v", err)
	}
	var pairs []monoapi.CurrencyPair
	if err := json.Unmarshal(b, &pairs); err != nil {
		log.Fatalf("unmarshal rates: %v", err)
	}

	type row struct{ pair, sell, buy, cross, date string }
	rows := make([]row, 0, len(pairs))
	for _, p := range pairs {
		rows = append(rows, row{
			pair:  fmt.Sprintf("%s/%s", sym(p.CurrencyCodeA), sym(p.CurrencyCodeB)),
			sell:  rateCell(p.RateSell),
			buy:   rateCell(p.RateBuy),
			cross: rateCell(p.RateCross),
			date:  time.Unix(p.Date, 0).Format("2006-01-02 15:04"),
		})
	}

	// Column widths from data plus the header labels.
	widths := []int{6, 6, 6, 6, 16}
	headers := []string{"pair", "sell", "buy", "cross", "updated"}
	for _, r := range rows {
		for i, c := range []string{r.pair, r.sell, r.buy, r.cross, r.date} {
			if n := len([]rune(c)); n > widths[i] {
				widths[i] = n
			}
		}
	}

	// Border rows: dashes span the full inner width; cell separators
	// are drawn as "+" at every column boundary.
	line := "+" + strings.Repeat("-", sum(widths)+len(widths)*2) + "+"
	fmt.Println(line)
	fmt.Println(rowOf(headers, widths) + "|")
	fmt.Println(line)
	for _, r := range rows {
		fmt.Println(rowOf([]string{r.pair, r.sell, r.buy, r.cross, r.date}, widths) + "|")
	}
	fmt.Println(line)
}

// sym maps a numeric ISO 4217 code to its symbol, with numeric fallback.
func sym(code int) string {
	if s, ok := currencySymbols[code]; ok {
		return s
	}
	return fmt.Sprintf("%d", code)
}

// rowOf renders one table row: cells padded to their column width
// (text columns left-aligned, numeric right-aligned), with "|"
// separators between columns. The caller adds the outer borders.
func rowOf(cells []string, widths []int) string {
	var b strings.Builder
	for i, c := range cells {
		b.WriteString("| ")
		if i == 0 || i == len(cells)-1 {
			// Left-align text columns, right-align rate columns.
			b.WriteString(c)
			b.WriteString(strings.Repeat(" ", widths[i]-len([]rune(c))))
		} else {
			b.WriteString(strings.Repeat(" ", widths[i]-len([]rune(c))))
			b.WriteString(c)
		}
		b.WriteString(" ")
	}
	return b.String()
}

// sum returns the total of the column widths.
func sum(w []int) int {
	s := 0
	for _, n := range w {
		s += n
	}
	return s
}

// printStatementSummary prints per-transaction lines for the statement
// call instead of the raw JSON blob.
func printStatementSummary(out *mcp.CallToolResult, fromUnix, toUnix int64) {
	fmt.Printf("\nmono_statement [last month %s .. %s]\n",
		time.Unix(fromUnix, 0).Format(time.DateOnly),
		time.Unix(toUnix, 0).Format(time.DateOnly))
	if out.IsError {
		b, _ := json.Marshal(out.StructuredContent)
		fmt.Printf("  tool error: %s\n", string(b))
		return
	}
	b, err := json.Marshal(out.StructuredContent)
	if err != nil {
		log.Fatalf("marshal statement: %v", err)
	}
	var items []monoapi.StatementItem
	if err := json.Unmarshal(b, &items); err != nil {
		log.Fatalf("unmarshal statement: %v", err)
	}
	fmt.Printf("  %d transactions in period\n", len(items))
	for _, it := range items {
		if len(items) > 10 {
			// Too many to print all: show first 10 and stop.
			if items[9].ID == it.ID {
				fmt.Println("  …")
				break
			}
		}
		fmt.Printf("  %s  %10.2f  %s%s\n",
			time.Unix(it.Time, 0).Format("2006-01-02 15:04"),
			float64(it.Amount)/100,
			it.Description,
			map[bool]string{true: " (hold)", false: ""}[it.Hold],
		)
	}
}
