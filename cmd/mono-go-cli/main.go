// Command mono-go-cli is a terminal client for the monobank open API
// (https://api.monobank.ua/docs/index.html).
//
// With no flags it prints: currency rates, bank sync info, and — with
// MONO_TOKEN (or a .env file) — client info plus two statement tables
// for the default account: from the 1st of last month to the 1st of
// this month, and from the 1st of this month to now.
//
// Flags select individual parts:
//
//	-rates        currency rates table
//	-sync         bank public key + server time table
//	-info         client info table (needs MONO_TOKEN)
//	-stmt         statement tables, both windows (needs MONO_TOKEN)
//	-account ID   statement account ("0" = default, or account/jar ID)
//	-webhook URL  set the webhook URL (POST /personal/webhook)
//	-no-wait      fail instead of waiting out the 60 s rate limit
//	-version      print version and exit
//
// Mind the 60-second rate limit on /personal/* endpoints: the two
// statement windows are separate calls, so the CLI waits between them
// (X-Auth-Interval-Expires, capped at 60 s) and retries once.
//
// The same parts are exposed as MCP tools by cmd/mono-go-mcp; this
// CLI exercises the monoapi layer directly (no MCP involved).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/dimetron/mono-go-mcp/internal/monoapi"
	"github.com/joho/godotenv"
)

// version is stamped at build time via -X main.version (goreleaser and
// the Taskfile CLI build); keep it a var or the stamp silently stops
// working.
var version = "0.1.0"

// options holds the selected CLI parts and modifiers.
type options struct {
	rates   bool
	sync    bool
	info    bool
	stmt    bool
	account string
	webhook string
	noWait  bool
}

func main() {
	os.Exit(run(os.Args[1:]))
}

// run parses args, executes the selected parts against the monobank API
// and returns the process exit code (0 success, 1 any selected part
// failed). Fatal conditions (bad webhook setup, statement fetch error)
// terminate via log.Fatal* inside this function.
func run(args []string) int {
	opts := options{account: "0"}
	var showVersion bool
	fs := flag.NewFlagSet("mono-go-cli", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.BoolVar(&opts.rates, "rates", false, "show currency rates table")
	fs.BoolVar(&opts.sync, "sync", false, "show bank public key + server time table")
	fs.BoolVar(&opts.info, "info", false, "show client info table (needs MONO_TOKEN)")
	fs.BoolVar(&opts.stmt, "stmt", false, "show statement tables for last month and this month (needs MONO_TOKEN)")
	fs.StringVar(&opts.account, "account", "0", "statement account: \"0\" for default, or an account/jar ID from client info")
	fs.StringVar(&opts.webhook, "webhook", "", "set the webhook URL that receives statement events (POST /personal/webhook)")
	fs.BoolVar(&opts.noWait, "no-wait", false, "do not wait out the 60 s personal-endpoint rate limit; fail with 429 instead")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if showVersion {
		fmt.Println("mono-go-cli", version)
		return 0
	}

	// No part selected: run everything.
	if !opts.rates && !opts.sync && !opts.info && !opts.stmt && opts.webhook == "" {
		opts.rates, opts.sync, opts.info, opts.stmt = true, true, true, true
	}

	_ = godotenv.Load(".env")
	token := os.Getenv("MONO_TOKEN")

	client := monoapi.NewClient(token, os.Getenv("MONO_BASE_URL"))
	ctx := context.Background()

	// failed counts the selected parts that could not be fetched;
	// main exits nonzero if any of them failed.
	failed := 0

	if opts.webhook != "" {
		if token == "" {
			log.Print("-webhook requires MONO_TOKEN")
			return 1
		}
		if err := client.SetWebHook(ctx, opts.webhook); err != nil {
			log.Printf("set webhook: %v", err)
			return 1
		}
		fmt.Printf("webhook set: %s\n", opts.webhook)
		if !opts.rates && !opts.sync && !opts.info && !opts.stmt {
			return 0
		}
	}

	if opts.rates {
		pairs, err := client.GetCurrencyRates(ctx)
		if err != nil {
			log.Printf("currency rates: %v", err)
			failed++
		} else {
			printRates(pairs)
		}
	}

	if opts.sync {
		si, err := client.GetBankSync(ctx)
		if err != nil {
			log.Printf("bank sync: %v", err)
			failed++
		} else {
			printBankSync(si)
		}
	}

	if !opts.info && !opts.stmt {
		return exitIfFailed(failed)
	}
	if token == "" {
		// An explicit -info/-stmt request that cannot run is an
		// error; the default no-flags invocation merely skips the
		// personal parts.
		if opts.info || opts.stmt {
			log.Print("no MONO_TOKEN — required for -info/-stmt (set it in the environment or .env)")
			return 1
		}
		fmt.Println("\nno MONO_TOKEN — personal parts skipped (set it in the environment or .env)")
		return exitIfFailed(failed)
	}

	if opts.info {
		ci, err := client.GetClientInfo(ctx)
		if err != nil {
			log.Printf("client info: %v", err)
			failed++
		} else {
			printClientInfo(ci)
		}
	}

	if opts.stmt {
		// All UTC: statement timestamps are Unix seconds,
		// timezone-independent. Mixing local calendar fields with a UTC
		// location can invert the window in the first hours of the 1st
		// (now < firstOfThisMonth), hence Now().UTC() for both.
		now := time.Now().UTC()
		// Last month: 1st of the previous month .. 1st of this month.
		// This month: 1st of this month .. now. Month windows are at
		// most 31 days, within the API's 2,682,000 s limit, so no cap
		// is needed.
		firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		firstOfLastMonth := firstOfThisMonth.AddDate(0, -1, 0)

		lastMonth, err := fetchStatement(ctx, client, opts.account, firstOfLastMonth, firstOfThisMonth, opts.noWait)
		if err != nil {
			log.Printf("statement: %v", err)
			return 1
		}
		printStatement("last month", firstOfLastMonth, firstOfThisMonth, lastMonth)

		thisMonth, err := fetchStatement(ctx, client, opts.account, firstOfThisMonth, now, opts.noWait)
		if err != nil {
			log.Printf("statement: %v", err)
			return 1
		}
		printStatement("this month", firstOfThisMonth, now, thisMonth)
	}

	return exitIfFailed(failed)
}

// exitIfFailed returns the process exit code: nonzero when some
// selected parts failed to be fetched. Statement fetches abort earlier
// via the error path, so reaching this point means every attempted
// part either printed or was counted in failed.
func exitIfFailed(failed int) int {
	if failed > 0 {
		return 1
	}
	return 0
}

// fetchStatement calls GetStatement, waiting out the 60 s rate limit
// once if the API answers 429 (unless noWait).
func fetchStatement(ctx context.Context, client *monoapi.Client, account string, from, to time.Time, noWait bool) (monoapi.StatementItems, error) {
	items, err := client.GetStatement(ctx, account, from.Unix(), to.Unix())
	if isRateLimited(err) && !noWait {
		wait, maxWait := rateLimitReset(err), int(time.Minute.Seconds())
		if wait <= 0 || wait > maxWait {
			wait = maxWait
		}
		fmt.Printf("\nrate limited; waiting %ds for the 60 s window to reset…\n", wait)
		select {
		case <-time.After(time.Duration(wait) * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		items, err = client.GetStatement(ctx, account, from.Unix(), to.Unix())
	}
	if err != nil {
		return nil, err
	}
	return items, nil
}

// isRateLimited reports whether err is a monobank 429.
func isRateLimited(err error) bool {
	apiErr, ok := err.(*monoapi.APIError)
	return ok && apiErr.IsRateLimited()
}

// rateLimitReset returns the server-provided reset seconds, or 0.
func rateLimitReset(err error) int {
	apiErr, ok := err.(*monoapi.APIError)
	if !ok {
		return 0
	}
	return apiErr.RateLimitResetSec
}

// printRates renders currency rates as a text table.
func printRates(pairs []monoapi.CurrencyPair) {
	fmt.Println("\nmono_currency_rates")
	rows := make([][]string, 0, len(pairs))
	for _, p := range pairs {
		rows = append(rows, []string{
			fmt.Sprintf("%s/%s", sym(p.CurrencyCodeA), sym(p.CurrencyCodeB)),
			rateCell(p.RateSell),
			rateCell(p.RateBuy),
			rateCell(p.RateCross),
			time.Unix(p.Date, 0).Format("2006-01-02 15:04"),
		})
	}
	printTable([]string{"pair", "sell", "buy", "cross", "updated"}, rows, []int{1, 2, 3})
}

// rateCell returns the printable cell for a rate: fixed precision or
// blank for the optional/absent cross rate.
func rateCell(r float64) string {
	if r == 0 {
		return ""
	}
	return fmt.Sprintf("%.4f", r)
}

// printBankSync prints mono_bank_sync data as a key/value table.
func printBankSync(si *monoapi.SyncInfo) {
	fmt.Println("\nmono_bank_sync")
	printTableAlign(
		[]string{"field", "value"},
		[][]string{
			{"serverKeyId", si.ServerKeyId},
			{"serverPubKey", si.ServerPubKey},
			{"serverTime", time.UnixMilli(si.ServerTimeMsec).Format(time.RFC3339)},
		},
		nil, // both columns are text: left-aligned
	)
}

// printClientInfo prints mono_client_info data as a table: one row
// per account and jar.
func printClientInfo(ci *monoapi.ClientInfo) {
	fmt.Printf("\nmono_client_info — %s (%s)\n", ci.Name, ci.ClientID)
	rows := make([][]string, 0, len(ci.Accounts)+len(ci.Jars))
	for _, a := range ci.Accounts {
		rows = append(rows, []string{
			"account", a.ID, a.Type, sym(a.CurrencyCode), fmtMoney(a.Balance),
		})
	}
	for _, j := range ci.Jars {
		rows = append(rows, []string{
			"jar", j.ID, j.Title, sym(j.CurrencyCode), fmtMoney(j.Balance),
		})
	}
	printTable([]string{"kind", "id", "name", "cur", "balance"}, rows, []int{4})
}

// printStatement prints one statement window as a transaction table
// with a totals line.
func printStatement(label string, from, to time.Time, items monoapi.StatementItems) {
	fmt.Printf("\nmono_statement [%s: %s .. %s]\n", label,
		from.Format(time.DateOnly), to.Format(time.DateOnly))
	if len(items) == 0 {
		fmt.Println("  no transactions in period")
		return
	}
	rows := make([][]string, 0, len(items))
	in, out := int64(0), int64(0)
	for _, it := range items {
		rows = append(rows, []string{
			time.Unix(it.Time, 0).Format("01-02 15:04"),
			fmtMoney(it.Amount),
			trunc(it.Description, 40),
			holdMark(it.Hold),
		})
		if it.Amount >= 0 {
			in += it.Amount
		} else {
			out += -it.Amount
		}
	}
	printTable([]string{"time", "amount", "description", "hold"}, rows, []int{1})
	fmt.Printf("  totals: %d transactions, in %s %s, out %s %s\n",
		len(items), fmtMoney(in), sym(items[0].CurrencyCode),
		fmtMoney(out), sym(items[0].CurrencyCode))
}

// holdMark renders the hold column.
func holdMark(hold bool) string {
	if hold {
		return "hold"
	}
	return ""
}

// trunc collapses all whitespace (including newlines) to single
// spaces, shortens s to at most n runes, and appends an ellipsis.
func trunc(s string, n int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "…"
}

// fmtMoney renders minimal currency units as major units.
func fmtMoney(v int64) string {
	return fmt.Sprintf("%.2f", float64(v)/100)
}

// printTable renders headers and rows as an aligned text table with
// box-drawing borders and | separators between all cells. rightAlign
// marks the indexes of right-aligned (numeric) columns; amounts,
// balances and rates should be right-aligned, text columns
// left-aligned. Every caller passes its own set: there is no
// one-size-fits-all numeric column.
func printTable(headers []string, rows [][]string, rightAlign []int) {
	printTableAlign(headers, rows, rightAlign)
}

// printTableAlign is printTable with a custom set of right-aligned
// columns (empty slice: all columns left-aligned).
func printTableAlign(headers []string, rows [][]string, rightAlign []int) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len([]rune(h))
	}
	for _, r := range rows {
		for i, c := range r {
			if n := len([]rune(c)); n > widths[i] {
				widths[i] = n
			}
		}
	}
	sep := "+"
	for _, w := range widths {
		sep += strings.Repeat("-", w+2) + "+"
	}
	fmt.Println(sep)
	printRow(headers, widths, rightAlign)
	fmt.Println(sep)
	for _, r := range rows {
		printRow(r, widths, rightAlign)
	}
	fmt.Println(sep)
}

// printRow renders one row; indexes in rightAlign are padded on the
// left (right-aligned), all other columns on the right.
func printRow(cells []string, widths []int, rightAlign []int) {
	right := map[int]bool{}
	for _, i := range rightAlign {
		right[i] = true
	}
	var b strings.Builder
	for i, c := range cells {
		b.WriteString("| ")
		if right[i] {
			b.WriteString(strings.Repeat(" ", widths[i]-len([]rune(c))))
			b.WriteString(c)
		} else {
			b.WriteString(c)
			b.WriteString(strings.Repeat(" ", widths[i]-len([]rune(c))))
		}
		b.WriteString(" ")
	}
	b.WriteString("|")
	fmt.Println(b.String())
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

// sym maps a numeric ISO 4217 code to its symbol, with numeric fallback.
func sym(code int) string {
	if s, ok := currencySymbols[code]; ok {
		return s
	}
	return fmt.Sprintf("%d", code)
}
