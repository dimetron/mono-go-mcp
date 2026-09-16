package monoapi

// This file mirrors the OpenAPI 3.0.3 schemas of the monobank open API
// (v250818). All monetary amounts are integers in minimal currency
// units (kopiykas/cents).

// CurrencyPair is one entry of GET /bank/currency.
type CurrencyPair struct {
	CurrencyCodeA int     `json:"currencyCodeA"` // ISO 4217 numeric, e.g. 840 (USD)
	CurrencyCodeB int     `json:"currencyCodeB"` // ISO 4217 numeric, e.g. 980 (UAH)
	Date          int64   `json:"date"`          // Unix seconds
	RateSell      float64 `json:"rateSell"`      // optional
	RateBuy       float64 `json:"rateBuy"`       // optional
	RateCross     float64 `json:"rateCross"`     // optional
}

// SyncInfo is the response of GET /bank/sync: the bank's public key
// (Secp256k1, uncompressed, Base64) and server time.
type SyncInfo struct {
	ServerKeyId    string `json:"serverKeyId"`    // hex
	ServerPubKey   string `json:"serverPubKey"`   // Base64
	ServerTimeMsec int64  `json:"serverTimeMsec"` // Unix milliseconds
}

// ClientInfo is the response of GET /personal/client-info.
type ClientInfo struct {
	ClientID    string    `json:"clientId"`             // same as id on send.monobank.ua
	Name        string    `json:"name"`                 // client name
	WebHookURL  string    `json:"webHookUrl,omitempty"` // current webhook, if set
	Permissions string    `json:"permissions"`          // one letter per permission, e.g. "psfj"
	Accounts    []Account `json:"accounts"`             // available accounts
	Jars        []Jar     `json:"jars,omitempty"`       // jars (goal buckets)
}

// Account is one bank account (card account).
type Account struct {
	ID           string   `json:"id"`
	SendID       string   `json:"sendId"`
	Balance      int64    `json:"balance"`      // minimal currency units
	CreditLimit  int64    `json:"creditLimit"`  // minimal currency units
	Type         string   `json:"type"`         // black|white|platinum|iron|fop|yellow|eAid
	CurrencyCode int      `json:"currencyCode"` // ISO 4217 numeric
	CashbackType string   `json:"cashbackType"` // None|UAH|Miles
	MaskedPan    []string `json:"maskedPan"`
	Iban         string   `json:"iban"`
}

// Jar is a "banka" — a goal bucket similar to a Piggy/savings jar.
type Jar struct {
	ID           string `json:"id"`
	SendID       string `json:"sendId"`
	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	CurrencyCode int    `json:"currencyCode"`
	Balance      int64  `json:"balance"`
	Goal         int64  `json:"goal"`
}

// StatementItems is the response of
// GET /personal/statement/{account}/{from}/{to}.
type StatementItems []StatementItem

// StatementItem is one transaction.
type StatementItem struct {
	ID              string `json:"id"`
	Time            int64  `json:"time"` // Unix seconds
	Description     string `json:"description"`
	MCC             int    `json:"mcc"`         // ISO 18245 merchant category code
	OriginalMcc     int    `json:"originalMcc"` // original MCC
	Hold            bool   `json:"hold"`
	Amount          int64  `json:"amount"`          // account currency, minimal units
	OperationAmount int64  `json:"operationAmount"` // operation currency, minimal units
	CurrencyCode    int    `json:"currencyCode"`    // ISO 4217 numeric
	CommissionRate  int64  `json:"commissionRate"`
	CashbackAmount  int64  `json:"cashbackAmount"`
	Balance         int64  `json:"balance"`
	Comment         string `json:"comment,omitempty"`
	ReceiptID       string `json:"receiptId,omitempty"`
	InvoiceID       string `json:"invoiceId,omitempty"`     // FOP income
	CounterEdrpou   string `json:"counterEdrpou,omitempty"` // FOP counterparty
	CounterIban     string `json:"counterIban,omitempty"`   // FOP counterparty
	CounterName     string `json:"counterName,omitempty"`   // counterparty name
}

// SetWebHook is the request body of POST /personal/webhook.
type SetWebHook struct {
	WebHookURL string `json:"webHookUrl"`
}
