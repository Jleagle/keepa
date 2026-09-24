# Keepa Go Client Library: Design

**Date:** 2026-09-24
**Status:** Approved 2026-09-24
**Module:** `github.com/Jleagle/keepa`
**Origin:** Extracted from `github.com/Jleagle/price-spider/keepa`

## 1. Purpose

A standalone, open-source Go client for the [Keepa API](https://keepa.com/api-docs/).
It replaces the `keepa` package inside price-spider. Every capability of that
package is preserved, but the API is modernised: context-first methods,
required arguments as parameters, everything else as typed functional options,
no globals, no logging framework dependency, and no database dependency.

Migrating price-spider onto this module is a separate follow-up and is out of
scope. Appendix A records the mapping so that follow-up is mechanical.

## 2. Goals and non-goals

**Goals**

- Feature parity with the current package (seven endpoints, predictive token
  bucket, envelope-before-status handling, typed API errors, token callback,
  pluggable limiter, full `Product` model, positional `csv` decoding).
- Callers control cancellation and deadlines through `context.Context`.
- Every value the current package hard-codes into a request becomes an
  explicit option. Options default to Keepa's own defaults, meaning the
  parameter is omitted from the request.
- A configurable token floor (reserve) that background work honours, with a
  per-call override so interactive work is not starved, automatic seeding of
  the bucket from Keepa's free token endpoint, and a non-blocking mode for
  queue consumers.
- Zero third-party dependencies.
- Unit tests for every endpoint, the token bucket, and every helper type,
  run with the race detector in CI.

**Non-goals**

- Endpoints the current package does not use: product finder, seller finder,
  tracking, notifications, graph images, category search, top sellers, and
  the storefront, offers, stock, A+ and code-lookup variants of the endpoints
  it does use. The option pattern makes them easy to add later.
- Token-cap awareness and surplus reporting (discussed, deferred).
- Persisting token state between processes.
- Migrating price-spider.

## 3. Global constraints

| Constraint | Value |
|---|---|
| Module path | `github.com/Jleagle/keepa` |
| Package name | `keepa` |
| Go directive | `go 1.26` |
| Dependencies | none outside the standard library |
| Licence | MIT, "Copyright (c) 2026 James Eagle" |
| Default base URL | `https://api.keepa.com` |
| Default request timeout | 60 seconds, applied only when the caller's context has no deadline; best sellers uses double |
| Default token reserve | 20 |
| Formatting | `gofmt` clean, `go vet` clean |
| Commits | none until the repository owner asks |

## 4. Repository layout

```
keepa/
  .github/workflows/test.yml   gofmt check, go vet, go test -race, on push to main and on pull requests
  .gitignore                   .idea/
  LICENSE                      MIT
  README.md                    install, quick start, options, token accounting, endpoint table, errors
  go.mod                       module github.com/Jleagle/keepa, go 1.26
  doc.go                       package documentation
  keepa.go                     Client, Option, NewClient, client options
  domain.go                    Domain enum and helpers
  time.go                      Time (Keepa minutes) and History
  csv.go                       CSVType enum, CSV struct and positional decoder
  errors.go                    APIError, HTTPError, TokenWaitError, sentinels
  transport.go                 request execution, envelope handling, Envelope type
  tokens.go                    token bucket, TokenState, TokenUpdate, seeding, GetTokenStatus
  options.go                   CallOption, WithReserve, WithoutWaiting, per-endpoint option plumbing
  product.go                   GetProducts and its options
  product_types.go             Product, ProductStats, Image, Variation, Video, FBAFees, CategoryNode
  bestsellers.go               GetBestSellers, options, response types
  category.go                  GetCategories, options, response types
  deals.go                     GetDeals, options, enums, response types
  lightning_deals.go           GetLightningDeals, response types
  search.go                    SearchProducts, options, response types
  seller.go                    GetSellers, response types
  *_test.go                    one test file per source file
  testdata/*.json              one fixture per endpoint plus error and token fixtures
  docs/superpowers/specs/      this document
  docs/superpowers/plans/      implementation plan
```

Each file has one responsibility. Response structs live next to the method
that returns them, except `Product` and its satellites, which are large
enough to earn their own file.

## 5. Client

```go
type Client struct { /* unexported */ }

type Option func(*Client)

func NewClient(apiKey string, opts ...Option) *Client
```

The API key is required, so it is a parameter. `NewClient` never fails.

| Option | Default | Effect |
|---|---|---|
| `WithHTTPClient(*http.Client)` | `&http.Client{}` | Transport used for every request. It carries no timeout of its own: every request gets a context deadline from `WithTimeout`, and a client-level timeout would silently cap the doubled best sellers deadline |
| `WithBaseURL(string)` | `https://api.keepa.com` | Trailing slash is trimmed |
| `WithLimiter(Limiter)` | none | Called before the token wait on every request |
| `WithLogger(*slog.Logger)` | `slog.New(slog.DiscardHandler)` | Receives the events listed in section 9 |
| `WithTokenCallback(func(TokenUpdate))` | none | Called once per Keepa envelope received, including error envelopes |
| `WithTokenReserve(int)` | 20 | Tokens to keep in hand; the floor |
| `WithTimeout(time.Duration)` | 60s | Fallback per-request deadline when the caller's context has none; zero or negative disables the fallback, as Go convention expects |

```go
// Limiter is satisfied by *golang.org/x/time/rate.Limiter without importing it.
type Limiter interface {
	Wait(ctx context.Context) error
}
```

There is no package-level default client. Callers that want one declare it.

## 6. Domain

```go
type Domain int

const (
	DomainUS Domain = 1  // amazon.com
	DomainGB Domain = 2  // amazon.co.uk
	DomainDE Domain = 3  // amazon.de
	DomainFR Domain = 4  // amazon.fr
	DomainJP Domain = 5  // amazon.co.jp
	DomainCA Domain = 6  // amazon.ca
	DomainIT Domain = 8  // amazon.it
	DomainES Domain = 9  // amazon.es
	DomainIN Domain = 10 // amazon.in
	DomainMX Domain = 11 // amazon.com.mx
	DomainBR Domain = 12 // amazon.com.br
)

func (d Domain) TLD() string      // "co.uk"; "" when invalid
func (d Domain) String() string   // "amazon.co.uk"; "Domain(7)" when invalid
func (d Domain) Valid() bool
func DomainFromTLD(tld string) (Domain, bool)
```

ID 7 is reserved by Keepa and is deliberately absent. Every endpoint
validates the domain and returns `ErrInvalidRequest` for an invalid one.

## 7. Time and History

Keepa timestamps are minutes since 2011-01-01 00:00 UTC, which is
21,564,000 minutes after the Unix epoch.

```go
type Time int

func (t Time) Time() time.Time        // UTC
func (t Time) Valid() bool            // t > 0; Keepa uses -1 and 0 for "unknown"
func (t Time) String() string         // RFC 3339, or "" when invalid
func TimeOf(t time.Time) Time         // truncates to the minute
```

`History` is a raw Keepa series with iterator helpers. Nothing is filtered
out; `-1` means "no data" in Keepa's convention and callers decide what to do
with it.

```go
type History []int

// Pairs iterates a [time, value, time, value, ...] series.
func (h History) Pairs() iter.Seq2[Time, int]

// Triples iterates a [time, price, shipping, ...] series.
func (h History) Triples() iter.Seq2[Time, PriceShipping]

type PriceShipping struct {
	Price    int
	Shipping int
}
```

A trailing incomplete group is ignored by both iterators.

## 8. CSV

Keepa's `csv` field is an array of up to 36 series indexed by price type.
The index becomes an exported enum so that `stats.Current[keepa.CSVAmazon]`
reads clearly.

```go
type CSVType int

const (
	CSVAmazon CSVType = iota // 0
	CSVNew                   // 1
	CSVUsed                  // 2
	CSVSales                 // 3
	CSVListPrice             // 4
	CSVCollectible           // 5
	CSVRefurbished           // 6
	CSVNewFBMShipping        // 7
	CSVLightningDeal         // 8
	CSVWarehouse             // 9
	CSVNewFBA                // 10
	CSVCountNew              // 11
	CSVCountUsed             // 12
	CSVCountRefurbished      // 13
	CSVCountCollectible      // 14
	CSVExtraInfoUpdates      // 15
	CSVRating                // 16
	CSVCountReviews          // 17
	CSVBuyBoxShipping        // 18
	CSVUsedNewShipping       // 19
	CSVUsedVeryGoodShipping  // 20
	CSVUsedGoodShipping      // 21
	CSVUsedAcceptableShipping // 22
	CSVCollectibleNewShipping // 23
	CSVCollectibleVeryGoodShipping // 24
	CSVCollectibleGoodShipping // 25
	CSVCollectibleAcceptableShipping // 26
	CSVRefurbishedShipping   // 27
	CSVEbayNewShipping       // 28
	CSVEbayUsedShipping      // 29
	CSVTradeIn               // 30
	CSVRental                // 31
	CSVBuyBoxUsedShipping    // 32
	CSVPrimeExclusive        // 33
	CSVCountNewFBA           // 34
	CSVCountNewFBM           // 35
)

func (t CSVType) String() string
func (t CSVType) Valid() bool
// HasShipping reports whether the series is [time, price, shipping] triples.
// True for 7, 18 through 29, and 32, per Keepa's product object documentation.
func (t CSVType) HasShipping() bool
```

`CSV` keeps every named field the current package has, typed as `History`,
in the same order, and keeps the positional decoder: `UnmarshalJSON` accepts
`[][]int` where an entry may be `null`, and fills the fields by index. Series
beyond the known range are ignored. `MarshalJSON` emits the same positional
array (nil series as `null`) so a decoded product round-trips through
`encoding/json`. A `Get(CSVType) History` accessor gives positional access
without a switch.

The BSON decoder is dropped. Nothing in price-spider persists `CSV`, and an
open-source client must not depend on a database driver.

## 9. Transport and envelope

Every Keepa response, including error responses, carries the same envelope:

```go
type Envelope struct {
	Timestamp          int64          `json:"timestamp"`          // Unix milliseconds
	TokensLeft         int            `json:"tokensLeft"`         // may be negative
	TokensConsumed     int            `json:"tokensConsumed"`
	RefillIn           int            `json:"refillIn"`           // milliseconds
	RefillRate         int            `json:"refillRate"`         // tokens per minute
	TokenFlowReduction float64        `json:"tokenFlowReduction"` // tokens per minute lost to tracking
	ProcessingTimeInMs int            `json:"processingTimeInMs"`
	Error              *APIError      `json:"error"`
}
```

Every response type embeds `Envelope`. After decoding, the transport sets
`Error.StatusCode` from the HTTP response, since Keepa does not include it in
the JSON.

**Execution order for one request**

1. Validate arguments. Return `ErrInvalidRequest` (wrapped with detail) before
   any network activity.
2. If the bucket has never been seeded, seed it (section 10). Failure is
   logged and ignored; the real response will sync the state anyway. The
   seed request itself skips this step.
3. If a `Limiter` is set, `Wait(ctx)`.
4. Token wait or non-blocking check (section 10). Requests with cost 0 skip
   this step.
5. If the context has no deadline and the resolved timeout is positive,
   derive one from it.
6. Send the request. GET endpoints put everything in the query string; the
   deals endpoint POSTs a JSON body with `Content-Type: application/json`.
   The client never sets `Accept-Encoding`; Go's transport negotiates gzip
   and decompresses transparently. A transport or URL error is rewrapped
   without the request URL, which carries the API key, keeping the cause
   available to `errors.Is`.
7. Read the body and decode the envelope. The response is treated as a Keepa
   envelope when `Timestamp > 0 || RefillRate > 0`. This is checked before
   the status code, because a 429 carries the authoritative token counts.
8. If it is an envelope and `RefillRate > 0`: update the bucket, invoke the
   token callback, and log a warning if `TokensConsumed` exceeds the expected
   cost. An envelope with `RefillRate == 0` carries no bucket reading (Keepa
   sends all-zero token fields with parameter errors) and is not recorded,
   otherwise the next paid call would wait on a fictional empty bucket. If
   `Error` is set, return `*APIError` with the HTTP status filled in.
9. If the status is not 200, return `*HTTPError`.
10. If the status is 200 but the body is valid JSON that is not an envelope,
    return an error naming the path; Keepa never answers that way, a proxy
    might.
11. Decode the typed response.

**Logging events** (all through the configured `*slog.Logger`, with the
`keepa:` prefix on messages):

| Level | Message | Attributes |
|---|---|---|
| Info | `keepa: waiting for tokens` | `cost`, `reserve`, `wait` |
| Warn | `keepa: token cost mismatch` | `path`, `expected`, `actual` |
| Warn | `keepa: token seed failed` | `error` |
| Warn | `keepa: undecodable response` | `status`, `body` (truncated to 512 bytes) |

## 10. Token accounting

Keepa grants `refillRate` tokens per minute, less `tokenFlowReduction`,
continuously, and the bucket holds at most sixty minutes of refill; tokens
older than that expire. The client keeps a projection of the bucket between
responses so it can pace itself without a round trip.

**State** (mutex-protected):

```go
type TokenState struct {
	Known         bool      // false until the first envelope or seed
	Left          int       // tokens at UpdatedAt
	RefillRate    int       // tokens per minute
	FlowReduction float64   // tokens per minute
	UpdatedAt     time.Time // in the future while calls are queued; Left is then the balance at that slot end after the queued spends
}

func (s TokenState) NetRefillRate() float64      // max(RefillRate - FlowReduction, 1)
func (s TokenState) Projected(at time.Time) float64

func (c *Client) Tokens() TokenState
```

**Wait rule.** For a call of cost `C` with reserve `R` at time `now`:

```
projected = Left + minutes(now - UpdatedAt) * NetRefillRate     // negative minutes if UpdatedAt is in the future
target    = C + R
if projected >= target:                                        // run immediately
    if UpdatedAt is in the future: Left -= C                   // the queue end stays where it is
    else:                          Left, UpdatedAt = projected - C, now
else:
    wait  = (target - projected) / NetRefillRate minutes
    runAt = now + wait
    if runAt is after UpdatedAt: Left, UpdatedAt = R, runAt    // this call becomes the queue end
    else:                        Left -= C                     // it runs before the queue end
    pending += C                                               // then sleep
```

The state `(Left, UpdatedAt)` always means "the balance at `UpdatedAt` after
every reservation made so far". Reserving under the mutex means concurrent
callers queue on a virtual timeline: each one waits for its own slot and they
run at exactly the net refill rate once the floor is reached. `pending` is
the summed cost of reservations still sleeping; it is released when a wait
elapses and the request is about to be sent. If the context is cancelled
during the sleep, `pending` is released and, if the slot is still in the
future at cancel time, `Left` is credited back by `C`; once the slot has
passed the tokens count as spent.

**Sync on response.** Each envelope overwrites `RefillRate` and
`FlowReduction`. If `UpdatedAt` is not in the future, `Left` becomes the
server's `tokensLeft` and `UpdatedAt` moves to `now`. If calls are queued,
the server's balance does not yet include their spends, so `Left` becomes
`tokensLeft - pending + minutes(UpdatedAt - now) * NetRefillRate`, the
balance at the slot end, and `UpdatedAt` stays. One approximation remains:
requests already sent whose envelope has not returned are not subtracted;
the window is one request's duration and that request's own envelope
corrects it.

**Reserve.** The client default is `WithTokenReserve`. Any call can override
it with `WithReserve(n)`, so background work honours the floor while an
interactive call passes `WithReserve(0)` and runs as long as the bucket can
pay for it.

**Seeding.** The first paid request on a client whose state is not `Known`
first calls `GET /token` (cost 0), so the floor applies from the very first
paid call. The seed request runs in its own goroutine with the initiator's
context stripped of cancellation (bounded by the fallback timeout), so one
caller giving up cannot fail the seed for everyone; every caller, including
the initiator, waits for it bounded by its own context, so a burst on a
fresh client cannot slip past the reserve. A failed seed is logged at Warn
and retried on the next paid call while the bucket is still unknown.
Requests with cost 0 never seed, which is why the status call cannot recurse.
`GetTokenStatus(ctx)` is also public for callers who want to inspect the
bucket.

**Non-blocking mode.** `WithoutWaiting()` on any call makes step 4 return
`*TokenWaitError` instead of sleeping, without reserving a slot. The error
carries the wait the client would have performed, so a queue consumer can
re-queue with that delay.

**Callback.** The token callback runs synchronously on the goroutine that
made the request, so it should return quickly.

**Clock.** The client holds an unexported `now func() time.Time` so tests can
drive projections deterministically. Sleeps use a real `time.Timer`; tests
that need a wait use large refill rates so the wait is milliseconds.

**Callback.**

```go
type TokenUpdate struct {
	Left          int
	Consumed      int
	RefillRate    int
	RefillIn      time.Duration
	FlowReduction float64
	Path          string    // request path, e.g. "/product"
	Timestamp     time.Time // server time from the envelope
}
```

## 11. Errors

```go
// APIError is an error object inside a Keepa envelope.
type APIError struct {
	StatusCode int    // HTTP status of the response that carried it
	Type       string // e.g. "invalidParameter"
	Message    string
	Details    string
}
func (e *APIError) Error() string
func (e *APIError) Is(target error) bool   // matches ErrNotEnoughTokens when StatusCode == 429

// HTTPError is a non-200 response that was not a Keepa envelope.
type HTTPError struct {
	StatusCode int
	Body       []byte
}
func (e *HTTPError) Error() string

// TokenWaitError is returned by calls made WithoutWaiting when the bucket
// cannot pay for the call without dropping below the reserve.
type TokenWaitError struct {
	Wait      time.Duration
	Cost      int
	Reserve   int
	Projected float64
}
func (e *TokenWaitError) Error() string
func (e *TokenWaitError) Is(target error) bool // matches ErrWouldWait

var (
	ErrInvalidRequest  = errors.New("keepa: invalid request")
	ErrNotEnoughTokens = errors.New("keepa: not enough tokens")
	ErrWouldWait       = errors.New("keepa: call would wait for tokens")
)
```

Argument validation errors wrap `ErrInvalidRequest` with a specific message,
for example `keepa: invalid request: between 1 and 100 ASINs required`.
Decoding failures of a 200 response are wrapped with the endpoint path.

## 12. Options plumbing

Each endpoint has its own option interface with an unexported apply method,
so endpoint-specific options cannot be passed to the wrong endpoint at compile
time. Cross-cutting call options implement every endpoint's interface through
a single exported interface:

```go
type ProductOption interface{ applyProduct(*productParams) }
type BestSellersOption interface{ applyBestSellers(*bestSellersParams) }
type CategoryOption interface{ applyCategory(*categoryParams) }
type DealsOption interface{ applyDeals(*dealsParams) }
type LightningDealsOption interface{ applyLightningDeals(*lightningDealsParams) }
type SearchOption interface{ applySearch(*searchParams) }
type SellersOption interface{ applySellers(*sellersParams) }

// CallOption applies to every endpoint.
type CallOption interface {
	ProductOption
	BestSellersOption
	CategoryOption
	DealsOption
	LightningDealsOption
	SearchOption
	SellersOption
}

func WithReserve(n int) CallOption
func WithoutWaiting() CallOption
```

Every `*Params` struct embeds an unexported `callParams{reserve *int; noWait bool}`.

## 13. Endpoints

All methods are on `*Client`, take `ctx` first, validate before any network
activity, and return a pointer to a response type embedding `Envelope`.

### 13.1 Products

```go
func (c *Client) GetProducts(ctx context.Context, domain Domain, asins []string, opts ...ProductOption) (*ProductResponse, error)
```

| Option | Query sent | Extra cost per ASIN |
|---|---|---|
| `WithStats(since time.Time)` | `stats=<sinceMs>,<nowMs>`; `since` earlier than 2011-01-01 is clamped to it | 0 |
| `WithRatings()` | `rating=1` | 1 |
| `WithLiveUpdate()` | `update=0` | 1 (Keepa may charge 0 if data is fresh) |
| `WithBuyBox()` | `buybox=1` | 2 |
| `WithVideos()` | `videos=1` | 0 |
| `WithoutHistory()` | `history=0` | 0 |

Always sent: `key`, `domain`, `asin` (comma-joined). Validation: 1 to 100
ASINs. Cost: `len(asins) × (1 + ratings + liveUpdate + 2 × buyBox)`.

```go
type ProductResponse struct {
	Envelope
	Products []Product `json:"products"`
}
```

### 13.2 Best sellers

```go
func (c *Client) GetBestSellers(ctx context.Context, domain Domain, categoryID int64, opts ...BestSellersOption) (*BestSellersResponse, error)
```

| Option | Query sent |
|---|---|
| `WithRankRange(days int)` | `range=<days>`; Keepa accepts 30, 90 or 180; validated |
| `WithVariations()` | `variations=1` |

Always sent: `key`, `domain`, `category`. Cost: 50. Fallback timeout is
doubled because the list is large.

```go
type BestSellersResponse struct {
	Envelope
	BestSellersList BestSellers `json:"bestSellersList"`
	ASINList        []string    `json:"asinList"`
}

type BestSellers struct {
	DomainID   int      `json:"domainId"`
	LastUpdate Time     `json:"lastUpdate"`
	CategoryID int64    `json:"categoryId"`
	ASINList   []string `json:"asinList"`
}

// ASINs returns the top-level list when present, otherwise the nested list.
func (r *BestSellersResponse) ASINs() []string
```

Keepa's documentation nests the list under `bestSellersList`, but the
current package reads a top-level `asinList` and receives ASINs in
production (confirmed by the owner). Both are decoded; the accessor prefers
the top-level list.

### 13.3 Categories

```go
func (c *Client) GetCategories(ctx context.Context, domain Domain, categoryIDs []int64, opts ...CategoryOption) (*CategoryResponse, error)
```

| Option | Query sent |
|---|---|
| `WithParents()` | `parents=1` |

Always sent: `key`, `domain`, `category` (comma-joined), and `parents`.
Keepa documents `parents` as required, so `parents=0` is sent when the option
is absent; this is the one deliberate exception to the omit-by-default rule.
Validation: 1 to 10 IDs. Cost: 1.

```go
type CategoryResponse struct {
	Envelope
	Categories      map[int64]Category `json:"categories"`
	CategoryParents map[int64]Category `json:"categoryParents"`
}
```

`Category` carries over every field the current package has, unchanged.

### 13.4 Deals

```go
func (c *Client) GetDeals(ctx context.Context, domain Domain, priceType CSVType, opts ...DealsOption) (*DealsResponse, error)
```

Keepa requires exactly one price type, so it is a parameter. The request is
a POST with a JSON body containing only `domainId`, `priceTypes: [priceType]`,
`page`, and whatever options set.

| Option | Body field |
|---|---|
| `WithDealsPage(n int)` | `page` (default 0) |
| `WithDealsDateRange(r DealsDateRange)` | `dateRange` |
| `WithDealsMinRating(r int)` | `minRating` (0 to 50) |
| `WithDealsOutOfStock()` | `isOutOfStock: true` |
| `WithDealsFilterErotic()` | `filterErotic: true` |
| `WithDealsSort(s DealsSort)` | `sortType`; negate a constant to invert the order |
| `WithDealsDeltaLastRange(min, max int)` | `deltaLastRange: [min, max]` |

```go
type DealsDateRange int
const (
	DealsDateRangeDay     DealsDateRange = 0
	DealsDateRangeWeek    DealsDateRange = 1
	DealsDateRangeMonth   DealsDateRange = 2
	DealsDateRangeQuarter DealsDateRange = 3
)

type DealsSort int
const (
	DealsSortAge          DealsSort = 1
	DealsSortDelta        DealsSort = 2
	DealsSortSalesRank    DealsSort = 3
	DealsSortDeltaPercent DealsSort = 4
)
```

Validation: `priceType` between 0 and 33. Cost: 5.

```go
type DealsResponse struct {
	Envelope
	Deals DealsPage `json:"deals"`
}

type DealsPage struct {
	Deals         []Deal   `json:"dr"`
	CategoryIDs   []int64  `json:"categoryIds"`
	CategoryNames []string `json:"categoryNames"`
	CategoryCount []int    `json:"categoryCount"`
}

type Deal struct {
	ASIN                      string  `json:"asin"`
	ParentASIN                string  `json:"parentAsin"`
	Title                     string  `json:"title"`
	RootCategory              int64   `json:"rootCat"`
	Categories                []int64 `json:"categories"`
	Image                     []int   `json:"image"` // ASCII codes; see ImageName
	Current                   []int   `json:"current"`
	CurrentSince              []Time  `json:"currentSince"`
	DeltaLast                 []int   `json:"deltaLast"`
	Delta                     [][]int `json:"delta"`        // [dateRange][priceType]
	DeltaPercent              [][]int `json:"deltaPercent"` // [dateRange][priceType]
	Avg                       [][]int `json:"avg"`          // [dateRange][priceType]
	LastUpdate                Time    `json:"lastUpdate"`
	CreationDate              Time    `json:"creationDate"`
	LightningEnd              Time    `json:"lightningEnd"`
	WarehouseCondition        int     `json:"warehouseCondition"`
	WarehouseConditionComment string  `json:"warehouseConditionComment"`
}

func (d Deal) ImageName() string
```

The current package's `Deal` type (AccessType, Badge, DealType) has no
producer anywhere and is dropped (open question 2).

### 13.5 Lightning deals

```go
func (c *Client) GetLightningDeals(ctx context.Context, domain Domain, opts ...LightningDealsOption) (*LightningDealsResponse, error)
```

No endpoint-specific options. Always sent: `key`, `domain`. Never sends an
empty `asin`, which Keepa rejects. Cost: 500.

```go
type LightningDealsResponse struct {
	Envelope
	LightningDeals []LightningDeal `json:"lightningDeals"`
}

type LightningDeal struct {
	DomainID            int                  `json:"domainId"`
	LastUpdate          Time                 `json:"lastUpdate"`
	ASIN                string               `json:"asin"`
	Title               string               `json:"title"`
	SellerID            string               `json:"sellerId"`
	SellerName          string               `json:"sellerName"`
	DealID              string               `json:"dealId"`
	DealPrice           int                  `json:"dealPrice"`    // -1 if upcoming
	CurrentPrice        int                  `json:"currentPrice"`
	Image               string               `json:"image"`
	IsPrimeEligible     bool                 `json:"isPrimeEligible"`
	IsFulfilledByAmazon bool                 `json:"isFulfilledByAmazon"`
	Rating              int                  `json:"rating"` // 0 to 50
	TotalReviews        int                  `json:"totalReviews"`
	DealState           string               `json:"dealState"`
	StartTime           Time                 `json:"startTime"`
	EndTime             Time                 `json:"endTime"`
	PercentClaimed      int                  `json:"percentClaimed"`
	PercentOff          int                  `json:"percentOff"`
	Variation           []VariationAttribute `json:"variation"`
}
```

This replaces the current struct, whose `Price` field decodes a `price` key
that Keepa's documentation does not list (open question 3).

### 13.6 Search

```go
func (c *Client) SearchProducts(ctx context.Context, domain Domain, term string, opts ...SearchOption) (*SearchResponse, error)
```

| Option | Query sent |
|---|---|
| `WithASINsOnly()` | `asins-only=1` |

Always sent: `key`, `domain`, `type=product`, `term`. Validation: non-empty
term. Cost: 10.

```go
type SearchResponse struct {
	Envelope
	Products []Product `json:"products"`
	ASINList []string  `json:"asinList"`
}
```

### 13.7 Sellers

```go
func (c *Client) GetSellers(ctx context.Context, domain Domain, sellerIDs []string, opts ...SellersOption) (*SellersResponse, error)
```

No endpoint-specific options. Always sent: `key`, `domain`, `seller`
(comma-joined). Validation: 1 to 100 IDs. Cost: `len(sellerIDs)`.

```go
type SellersResponse struct {
	Envelope
	Sellers map[string]Seller `json:"sellers"`
}
```

`Seller` carries over every field the current package has; `LastUpdate` and
`TrackedSince` become `Time`, `AsinListLastSeen` becomes `[]Time`.

### 13.8 Token status

```go
func (c *Client) GetTokenStatus(ctx context.Context) (*TokenResponse, error)

type TokenResponse struct {
	Envelope
}
```

Always sent: `key`. Cost: 0, so it bypasses the token wait. Used for seeding.

## 14. Response models

`Product`, `ProductStats`, `Image`, `Variation`, `Video`, `FBAFees`,
`CategoryNode` and `Category` are carried over field for field from the
current package with these changes only:

- `CSV` fields are `History`.
- Fields Keepa documents as Keepa minutes become `Time`: on `Product`,
  `TrackingSince`, `ListedSince`, `LastUpdate`, `LastRatingUpdate`,
  `LastPriceChange`, `LastEbayUpdate`, `LastSoldUpdate`, `Reviews.LastUpdate`;
  on `ProductStats`, `LastOffersUpdate`. `PublicationDate` and `ReleaseDate`
  stay `int` (Keepa sends `YYYYMMDD`). Fields the current package types as
  `any` stay `any`.
- `Variation.Attributes` uses a named `VariationAttribute{Dimension, Value}`
  type, shared with `LightningDeal`.
- `Product.LastCategory()` is kept.

## 15. Testing

Every test uses `net/http/httptest`; nothing touches the network. A shared
helper starts a server that answers `GET /token` with a fixture and routes
other paths to a per-test handler that records the request.

| Area | Tests |
|---|---|
| Each endpoint | method, path, every query parameter or body field for defaults and for each option, ASIN and ID joining, validation errors before any request, decoding from `testdata/<endpoint>.json`, expected cost passed to the bucket |
| Transport | 429 envelope records tokens and returns `*APIError` matching `ErrNotEnoughTokens`; non-envelope 502 returns `*HTTPError` and does not touch tokens; cost mismatch logs a warning; token callback receives every field; fallback timeout applied only without a deadline |
| Token bucket | no wait when plentiful; wait computed from net rate; concurrent callers queue; refund on cancel; per-call reserve override; `WithoutWaiting` returns `*TokenWaitError` without reserving; seeding runs once and applies the floor to the first paid call; seed failure is logged and ignored; cost 0 bypasses the wait; `Projected` maths |
| Domain | TLD, String, Valid, DomainFromTLD, reserved ID 7 |
| Time | round trip with the 21,564,000 offset; Valid; String |
| History | Pairs and Triples, including empty and trailing incomplete groups |
| CSV | positional decode with nulls and short arrays; `Get`; `HasShipping` set |
| Errors | `Error()` strings; `errors.Is` and `errors.As` behaviour |

Fixtures are hand-written from Keepa's documented shapes and the real error
envelope captured from the token endpoint. They exist to pin decoding, not
to reproduce production data.

CI: `.github/workflows/test.yml` runs on push to `main` and on pull
requests: checkout, setup-go from `go.mod`, gofmt check, `go vet ./...`,
`go test -race -count=1 ./...`.

## 16. README outline

1. Badges: pkg.go.dev, CI.
2. One-paragraph description and the "no dependencies" note.
3. Install.
4. Quick start: client with a token callback and a reserve, one product call
   with options, error handling with `errors.As` and `errors.Is`.
5. Endpoint table with token costs.
6. Token accounting: how the floor works, per-call override, non-blocking
   mode, seeding, `Tokens()`.
7. Working with history: `Time`, `History`, `CSVType`.
8. Licence.

## 17. Open questions to settle during review

1. **Best sellers shape.** Settled: production receives ASINs through the
   top-level `asinList`. Both shapes stay decoded; the accessor prefers the
   top-level list.
2. **Unused `Deal` type.** The current package declares `Deal{AccessType,
   Badge, DealType}` but nothing produces or consumes it. It is dropped.
3. **Lightning deal price.** The current `LightningDeal.Price` decodes a
   `price` key that Keepa does not document; price-spider only reads `ASIN`.
   The new struct follows the documentation.

## Appendix A: migration map for price-spider (out of scope)

| Today | New module |
|---|---|
| `regions.KeepaDomainInt` | `keepa.Domain` (same integer values; an alias `type KeepaDomainInt = keepa.Domain` keeps regions compiling) |
| `keepa.DefaultClient` | a variable in price-spider, e.g. `var Keepa *keepa.Client` |
| `NewClient(WithAPIKey(k), WithTokensCallback(fn))` | `NewClient(k, WithTokenCallback(fn), WithTokenReserve(n), WithLogger(l))` |
| `GetProducts(d, asins, since, true, live, buyBox)` | `GetProducts(ctx, d, asins, WithStats(since), WithRatings(), WithVideos())`, adding `WithLiveUpdate()` and `WithBuyBox()` when the corresponding bools were true |
| `GetBestSellers(d, id)` | `GetBestSellers(ctx, d, id)` then `resp.ASINs()` |
| `GetCategoryLookup(d, ids)` | `GetCategories(ctx, d, ids, WithParents())` |
| `GetDeals(d)` | `GetDeals(ctx, d, keepa.CSVAmazon, WithDealsMinRating(35), WithDealsFilterErotic(), WithDealsSort(keepa.DealsSortSalesRank), WithDealsDeltaLastRange(0, math.MaxInt32))` |
| `GetLightningDeals(d)` | `GetLightningDeals(ctx, d)` |
| `SearchProducts(d, term)` | `SearchProducts(ctx, d, term, WithASINsOnly())` |
| `GetSeller(d, id)` | `GetSellers(ctx, d, []string{id})` |
| `parseKeepaArray` | `History.Pairs()` and `Time.Time()` |
| `zap.L()` inside the package | `WithLogger` with a `slog.Handler` that forwards to zap |
