# keepa

[![Go Reference](https://pkg.go.dev/badge/github.com/Jleagle/keepa.svg)](https://pkg.go.dev/github.com/Jleagle/keepa)
[![Test](https://github.com/Jleagle/keepa/actions/workflows/test.yml/badge.svg)](https://github.com/Jleagle/keepa/actions/workflows/test.yml)

A Go client for the [Keepa API](https://keepa.com/api-docs/).

- Context-first methods; required arguments as parameters, everything else as options
- Tracks your token bucket between calls and keeps a configurable reserve, so background work paces itself to your refill rate and interactive calls still get through
- Typed errors you can test with `errors.Is` and `errors.As`
- No dependencies

## Install

```sh
go get github.com/Jleagle/keepa
```

## Usage

```go
client := keepa.NewClient(apiKey, keepa.WithTokenReserve(1000))

resp, err := client.GetProducts(ctx, keepa.DomainUS, []string{"B07XJ8C8F5"},
    keepa.WithStats(time.Now().AddDate(0, -1, 0)),
    keepa.WithRatings(),
)
if err != nil {
    var apiErr *keepa.APIError
    switch {
    case errors.Is(err, keepa.ErrNotEnoughTokens):
        // quota exhausted
    case errors.As(err, &apiErr):
        // apiErr.Type, apiErr.Message, apiErr.StatusCode
    }
}

for _, p := range resp.Products {
    fmt.Println(p.ASIN, p.Title, p.LastUpdate.Time())
    for at, price := range p.CSV.Amazon.Pairs() {
        fmt.Println(at.Time(), price) // -1 means no offer
    }
}
```

## Endpoints

| Method | Required | Options | Tokens |
|---|---|---|---|
| `GetProducts` | domain, ASINs (1 to 100) | `WithStats`, `WithRatings`, `WithLiveUpdate`, `WithBuyBox`, `WithVideos`, `WithoutHistory` | 1 per ASIN, +1 ratings, +1 live update, +2 buy box |
| `GetBestSellers` | domain, category ID | `WithRankRange`, `WithVariations` | 50 |
| `GetCategories` | domain, category IDs (1 to 10) | `WithParents` | 1 |
| `GetDeals` | domain, price type | `WithDealsPage`, `WithDealsDateRange`, `WithDealsMinRating`, `WithDealsOutOfStock`, `WithDealsFilterErotic`, `WithDealsSort`, `WithDealsDeltaLastRange` | 5 |
| `GetLightningDeals` | domain | | 500 |
| `SearchProducts` | domain, term | `WithASINsOnly` | 10 |
| `GetSellers` | domain, seller IDs (1 to 100) | | 1 per seller |
| `GetTokenStatus` | | | 0 |

Options default to Keepa's own defaults, meaning the parameter is not sent,
except `parents` and `page`, which Keepa requires and are always sent.

## Client options

| Option | Default |
|---|---|
| `WithHTTPClient(*http.Client)` | no timeout of its own; each request carries a deadline (see `WithTimeout`) |
| `WithBaseURL(string)` | `https://api.keepa.com` |
| `WithLimiter(Limiter)` | none; `*rate.Limiter` from `golang.org/x/time/rate` fits |
| `WithLogger(*slog.Logger)` | discard |
| `WithTokenCallback(func(TokenUpdate))` | none; called for every Keepa envelope, including errors |
| `WithTokenReserve(int)` | 20 |
| `WithTimeout(time.Duration)` | 1 minute, applied only when your context has no deadline; 0 disables it |

## Token accounting

Keepa refills your bucket continuously at your plan's rate and holds at most
sixty minutes of tokens, so unused tokens are lost. Every response reports the
bucket, and the client projects it forward between responses.

Before each call the client checks whether the projected bucket can pay the
call's cost while keeping the reserve. If not, it sleeps exactly until it can.
At the floor, calls self-pace to your refill rate; above it, they run flat out
until the surplus is spent. Concurrent callers queue on a shared timeline.

- `WithTokenReserve(n)` sets the floor for every call.
- `WithReserve(n)` on a call overrides it. Give background jobs the client
  floor and interactive calls `WithReserve(0)`.
- `WithoutWaiting()` on a call returns a `*TokenWaitError` (matching
  `ErrWouldWait`) with the wait it would have performed, so a queue consumer
  can re-queue instead of blocking. The same error comes back immediately
  when the wait would outlive your context's deadline, so a short deadline
  never holds a worker for its full length only to fail.
- The first paid call fetches `GetTokenStatus` (free) so the floor applies
  immediately. `client.Tokens()` returns a snapshot of the bucket;
  `Tokens().Projected(time.Now())` is the current estimate.

## Errors

- `*APIError`: the error Keepa put in the response envelope, with `Type`, `Message` and `StatusCode`; matches `ErrNotEnoughTokens` on a 429.
- `*HTTPError`: a non-200 response that was not a Keepa envelope, with `StatusCode` and `Body`.
- `*TokenWaitError`: a `WithoutWaiting()` call that would have slept, with `Wait`; matches `ErrWouldWait`.
- `ErrInvalidRequest`: wraps every argument validation failure, returned before any request is sent.

The client never logs or returns your API key: transport errors have the
request URL stripped.

## Working with history

Keepa timestamps are minutes since 2011. `keepa.Time` converts them with
`.Time()`. Price and rank series are `keepa.History` values with `Pairs()`
for `[time, value]` series and `Triples()` for the shipping-inclusive series
(`CSVType.HasShipping` tells you which). `CSVType` constants index the
statistics arrays: `stats.Current[keepa.CSVAmazon]`.

## Licence

MIT
