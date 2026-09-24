# Keepa Go Client Library Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `github.com/Jleagle/keepa`, a dependency-free Go client for the Keepa API with context-first endpoint methods, typed functional options, and a predictive token bucket with a configurable floor.

**Architecture:** One flat package `keepa`. A `Client` holds configuration and a mutex-protected token bucket. Every endpoint validates its arguments, builds a `request` (path, query or JSON body, expected token cost, per-call policy) and hands it to a generic `do[T]` that applies the limiter, the token wait, the fallback timeout, sends the request, records the Keepa envelope from every response before checking the status code, and decodes the typed result. Per-endpoint options are interfaces with unexported apply methods; cross-cutting call options implement all of them.

**Tech Stack:** Go 1.26 standard library only (`net/http`, `encoding/json`, `log/slog`, `iter`, `net/http/httptest` for tests). CI on GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-24-keepa-library-design.md` (read it first; every task argues from it).

## Global Constraints

- Module path `github.com/Jleagle/keepa`, package `keepa`, `go 1.26` in `go.mod`.
- No dependencies outside the standard library. `go.sum` must not exist.
- Licence MIT, "Copyright (c) 2026 James Eagle".
- Default base URL `https://api.keepa.com`; default request timeout 60 seconds applied only when the caller's context has no deadline; best sellers doubles it; default token reserve 20.
- Every log message starts with `keepa: ` and goes through the client's `*slog.Logger`, which defaults to a discard handler.
- Every error message starts with `keepa: `.
- `gofmt -l .` prints nothing and `go vet ./...` is clean after every task.
- All tests run with `go test -race -count=1 ./...` and never touch the network.
- The repository owner has asked for no commits unless approved. Run each "Commit" step only if commits have been approved for this repository; otherwise skip it and continue.
- Shell note: `cat` is aliased to `bat` on the owner's machine; use `/bin/cat` or `sed -n`. Do not `cd`; use absolute paths under `/Users/jameseagle/code/Jleagle/keepa`.

## Review Focus

1. A `csv` array with `null` entries or fewer than 36 series must decode without panicking and leave the missing fields `nil` (Task 3 tests `TestCSVUnmarshalPositional`).
2. A `History` with an odd number of elements must iterate the complete groups and ignore the trailing element rather than panic (Task 2 test `TestHistoryPairs`).
3. Concurrent callers must queue on the bucket without racing and without both being admitted for the same tokens (Task 5 test `TestReserveTokensQueuesConcurrentCallers`, plus `-race` in every run).
4. A 200 response whose body is not JSON must return a wrapped decode error, not a panic, and must not modify token state (Task 6 test `TestDoUndecodable200`).
5. When `/token` itself fails, the seed must not recurse or retry in a loop within one call; the paid call proceeds and the next call seeds again (Task 7 tests `TestSeedFailureIsLoggedAndIgnored`, `TestSeedDoesNotRecurse`).

---

## File Structure

| File | Responsibility |
|---|---|
| `go.mod` | module path and Go version |
| `LICENSE`, `.gitignore`, `README.md`, `.github/workflows/test.yml` | repository scaffolding |
| `doc.go` | package documentation |
| `domain.go` | `Domain` enum, `TLD`, `String`, `Valid`, `DomainFromTLD` |
| `time.go` | `Time` (Keepa minutes), `History`, `PriceShipping` |
| `csv.go` | `CSVType` enum, `CSV` struct, positional `UnmarshalJSON`, `Get` |
| `errors.go` | `APIError`, `HTTPError`, `TokenWaitError`, sentinels, `invalidRequest` |
| `keepa.go` | `Client`, `Option`, `NewClient`, client options, `Limiter` |
| `options.go` | `callParams`, per-endpoint option interfaces, `CallOption`, `WithReserve`, `WithoutWaiting` |
| `tokens.go` | `TokenState`, `TokenUpdate`, bucket reserve/wait/refund, seeding, `GetTokenStatus` |
| `transport.go` | `Envelope`, `request`, `do[T]`, request building |
| `product.go`, `product_types.go` | `GetProducts`, its options, `Product` and satellites |
| `bestsellers.go`, `category.go`, `deals.go`, `lightning_deals.go`, `search.go`, `seller.go` | one endpoint each with its params, options and response types |
| `*_test.go`, `testhelpers_test.go`, `testdata/*.json` | tests and fixtures |

---

### Task 1: Module scaffold and Domain

**Files:**
- Create: `go.mod`, `LICENSE`, `.gitignore`, `doc.go`, `domain.go`
- Test: `domain_test.go`

**Interfaces:**
- Produces: `type Domain int`; constants `DomainUS`…`DomainBR`; `func (d Domain) TLD() string`; `func (d Domain) String() string`; `func (d Domain) Valid() bool`; `func DomainFromTLD(tld string) (Domain, bool)`; unexported `func (d Domain) queryValue() string` used by every endpoint.

- [ ] **Step 1: Create the module files**

`go.mod`:

```
module github.com/Jleagle/keepa

go 1.26
```

`.gitignore`:

```
.idea/
```

`LICENSE` (MIT, verbatim):

```
MIT License

Copyright (c) 2026 James Eagle

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

`doc.go`:

```go
// Package keepa is a Go client for the Keepa API, https://keepa.com/api-docs/.
//
// Create a client with NewClient and call the endpoint methods. Every method
// takes a context first, then the arguments Keepa requires, then options:
//
//	client := keepa.NewClient(apiKey, keepa.WithTokenReserve(1000))
//	resp, err := client.GetProducts(ctx, keepa.DomainUS, []string{"B07XJ8C8F5"},
//		keepa.WithStats(time.Now().AddDate(0, -1, 0)), keepa.WithRatings())
//
// The client tracks Keepa's token bucket between responses and blocks a call
// that would drop the bucket below the reserve. See Client.Tokens,
// WithTokenReserve, WithReserve and WithoutWaiting.
package keepa
```

- [ ] **Step 2: Write the failing tests**

`domain_test.go`:

```go
package keepa

import (
	"strconv"
	"testing"
)

func TestDomainTLD(t *testing.T) {
	tests := []struct {
		domain Domain
		tld    string
		str    string
	}{
		{DomainUS, "com", "amazon.com"},
		{DomainGB, "co.uk", "amazon.co.uk"},
		{DomainDE, "de", "amazon.de"},
		{DomainFR, "fr", "amazon.fr"},
		{DomainJP, "co.jp", "amazon.co.jp"},
		{DomainCA, "ca", "amazon.ca"},
		{DomainIT, "it", "amazon.it"},
		{DomainES, "es", "amazon.es"},
		{DomainIN, "in", "amazon.in"},
		{DomainMX, "com.mx", "amazon.com.mx"},
		{DomainBR, "com.br", "amazon.com.br"},
	}
	for _, tt := range tests {
		if got := tt.domain.TLD(); got != tt.tld {
			t.Errorf("Domain(%d).TLD() = %q, want %q", tt.domain, got, tt.tld)
		}
		if got := tt.domain.String(); got != tt.str {
			t.Errorf("Domain(%d).String() = %q, want %q", tt.domain, got, tt.str)
		}
		if !tt.domain.Valid() {
			t.Errorf("Domain(%d).Valid() = false, want true", tt.domain)
		}
		if got := tt.domain.queryValue(); got != strconv.Itoa(int(tt.domain)) {
			t.Errorf("Domain(%d).queryValue() = %q", tt.domain, got)
		}
	}
}

func TestDomainInvalid(t *testing.T) {
	for _, d := range []Domain{0, 7, 13, -1} {
		if d.Valid() {
			t.Errorf("Domain(%d).Valid() = true, want false", d)
		}
		if got := d.TLD(); got != "" {
			t.Errorf("Domain(%d).TLD() = %q, want empty", d, got)
		}
	}
	if got := Domain(7).String(); got != "Domain(7)" {
		t.Errorf("Domain(7).String() = %q, want Domain(7)", got)
	}
}

func TestDomainFromTLD(t *testing.T) {
	d, ok := DomainFromTLD("co.uk")
	if !ok || d != DomainGB {
		t.Errorf("DomainFromTLD(co.uk) = %d, %v; want DomainGB, true", d, ok)
	}
	if _, ok := DomainFromTLD("cn"); ok {
		t.Error("DomainFromTLD(cn) found a domain; Keepa does not serve it")
	}
	if _, ok := DomainFromTLD(""); ok {
		t.Error("DomainFromTLD(\"\") found a domain")
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./... 2>&1 | head -20` from `/Users/jameseagle/code/Jleagle/keepa`
Expected: build failure mentioning `undefined: Domain`.

- [ ] **Step 4: Write the implementation**

`domain.go`:

```go
package keepa

import (
	"fmt"
	"strconv"
)

// Domain identifies an Amazon marketplace by Keepa's numeric domain ID.
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

// ID 7 is reserved by Keepa and deliberately absent.
var domainTLDs = map[Domain]string{
	DomainUS: "com",
	DomainGB: "co.uk",
	DomainDE: "de",
	DomainFR: "fr",
	DomainJP: "co.jp",
	DomainCA: "ca",
	DomainIT: "it",
	DomainES: "es",
	DomainIN: "in",
	DomainMX: "com.mx",
	DomainBR: "com.br",
}

// TLD returns the Amazon top-level domain, such as "co.uk", or "" when d is not valid.
func (d Domain) TLD() string { return domainTLDs[d] }

// Valid reports whether d is a marketplace Keepa serves.
func (d Domain) Valid() bool {
	_, ok := domainTLDs[d]
	return ok
}

// String returns the Amazon host name, such as "amazon.co.uk".
func (d Domain) String() string {
	if tld, ok := domainTLDs[d]; ok {
		return "amazon." + tld
	}
	return fmt.Sprintf("Domain(%d)", int(d))
}

// DomainFromTLD returns the Domain for an Amazon top-level domain such as "co.uk".
func DomainFromTLD(tld string) (Domain, bool) {
	for d, t := range domainTLDs {
		if t == tld {
			return d, true
		}
	}
	return 0, false
}

func (d Domain) queryValue() string { return strconv.Itoa(int(d)) }
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test -race -count=1 ./...`
Expected: `ok  	github.com/Jleagle/keepa`

- [ ] **Step 6: Commit**

```bash
git -C /Users/jameseagle/code/Jleagle/keepa add go.mod LICENSE .gitignore doc.go domain.go domain_test.go
git -C /Users/jameseagle/code/Jleagle/keepa commit -m "Add module scaffold and Domain type"
```

---

### Task 2: Time and History

**Files:**
- Create: `time.go`
- Test: `time_test.go`

**Interfaces:**
- Produces: `type Time int`; `func (t Time) Time() time.Time`; `func (t Time) Valid() bool`; `func (t Time) String() string`; `func TimeOf(t time.Time) Time`; `type History []int`; `func (h History) Pairs() iter.Seq2[Time, int]`; `func (h History) Triples() iter.Seq2[Time, PriceShipping]`; `type PriceShipping struct{ Price, Shipping int }`.

- [ ] **Step 1: Write the failing tests**

`time_test.go`:

```go
package keepa

import (
	"slices"
	"testing"
	"time"
)

func TestTimeConversion(t *testing.T) {
	// Keepa minute 0 is 2011-01-01T00:00:00Z.
	if got := Time(0).Time(); !got.Equal(time.Date(2011, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Time(0).Time() = %v", got)
	}
	// The consumer formula in price-spider: unix = (keepaMinutes + 21564000) * 60.
	want := time.Unix((1000+21564000)*60, 0).UTC()
	if got := Time(1000).Time(); !got.Equal(want) {
		t.Errorf("Time(1000).Time() = %v, want %v", got, want)
	}
	if got := Time(1000).Time().Location(); got != time.UTC {
		t.Errorf("Time().Location() = %v, want UTC", got)
	}
	if got := TimeOf(want); got != 1000 {
		t.Errorf("TimeOf(%v) = %d, want 1000", want, got)
	}
	if got := TimeOf(want.Add(59 * time.Second)); got != 1000 {
		t.Errorf("TimeOf did not truncate seconds: got %d, want 1000", got)
	}
}

func TestTimeValidAndString(t *testing.T) {
	for _, v := range []Time{0, -1} {
		if v.Valid() {
			t.Errorf("Time(%d).Valid() = true, want false", v)
		}
		if got := v.String(); got != "" {
			t.Errorf("Time(%d).String() = %q, want empty", v, got)
		}
	}
	if !Time(1).Valid() {
		t.Error("Time(1).Valid() = false, want true")
	}
	if got := Time(1000).String(); got != "2011-01-01T16:40:00Z" {
		t.Errorf("Time(1000).String() = %q, want 2011-01-01T16:40:00Z", got)
	}
}

func collectPairs(h History) (times []Time, values []int) {
	for tm, v := range h.Pairs() {
		times = append(times, tm)
		values = append(values, v)
	}
	return times, values
}

func TestHistoryPairs(t *testing.T) {
	// Trailing 30 has no value and must be ignored; -1 is passed through.
	times, values := collectPairs(History{10, 100, 20, -1, 30})
	if !slices.Equal(times, []Time{10, 20}) || !slices.Equal(values, []int{100, -1}) {
		t.Errorf("Pairs() = %v %v, want [10 20] [100 -1]", times, values)
	}
	if times, _ := collectPairs(nil); len(times) != 0 {
		t.Errorf("Pairs() on nil yielded %v", times)
	}
	if times, _ := collectPairs(History{5}); len(times) != 0 {
		t.Errorf("Pairs() on a single element yielded %v", times)
	}
}

func TestHistoryPairsStopsOnBreak(t *testing.T) {
	n := 0
	for range History{1, 2, 3, 4, 5, 6}.Pairs() {
		n++
		break
	}
	if n != 1 {
		t.Errorf("iteration continued after break: %d", n)
	}
}

func TestHistoryTriples(t *testing.T) {
	// Trailing group [30, 300] is incomplete and must be ignored.
	var times []Time
	var points []PriceShipping
	for tm, p := range (History{10, 100, 5, 20, 200, 0, 30, 300}).Triples() {
		times = append(times, tm)
		points = append(points, p)
	}
	want := []PriceShipping{{Price: 100, Shipping: 5}, {Price: 200, Shipping: 0}}
	if !slices.Equal(times, []Time{10, 20}) || !slices.Equal(points, want) {
		t.Errorf("Triples() = %v %v, want [10 20] %v", times, points, want)
	}
	n := 0
	for range History(nil).Triples() {
		n++
	}
	if n != 0 {
		t.Errorf("Triples() on nil yielded %d groups", n)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... 2>&1 | head -20`
Expected: build failure mentioning `undefined: Time` or `undefined: History`.

- [ ] **Step 3: Write the implementation**

`time.go`:

```go
package keepa

import (
	"iter"
	"time"
)

// keepaEpochMinutes is the number of minutes from the Unix epoch to
// 2011-01-01 00:00 UTC, the start of Keepa time.
const keepaEpochMinutes = 21564000

// Time is a Keepa timestamp: whole minutes since 2011-01-01 00:00 UTC.
// Keepa uses 0 and -1 where a timestamp is unknown; see Valid.
type Time int

// Time converts t to a time.Time in UTC.
func (t Time) Time() time.Time {
	return time.Unix((int64(t)+keepaEpochMinutes)*60, 0).UTC()
}

// Valid reports whether t holds a real timestamp.
func (t Time) Valid() bool { return t > 0 }

// String formats t as RFC 3339, or returns "" when t is not valid.
func (t Time) String() string {
	if !t.Valid() {
		return ""
	}
	return t.Time().Format(time.RFC3339)
}

// TimeOf converts a time.Time to Keepa minutes, truncating to the minute.
func TimeOf(t time.Time) Time {
	return Time(t.Unix()/60 - keepaEpochMinutes)
}

// History is a raw Keepa time series as sent in a product's csv field.
// Most series are [time, value, time, value, ...]; the shipping-inclusive
// price types are [time, price, shipping, ...]. See CSVType.HasShipping.
type History []int

// Pairs iterates a [time, value, ...] series. A trailing incomplete group is
// ignored. Values are not filtered; Keepa sends -1 for "no data".
func (h History) Pairs() iter.Seq2[Time, int] {
	return func(yield func(Time, int) bool) {
		for i := 0; i+1 < len(h); i += 2 {
			if !yield(Time(h[i]), h[i+1]) {
				return
			}
		}
	}
}

// PriceShipping is one point of a shipping-inclusive series.
type PriceShipping struct {
	Price    int
	Shipping int
}

// Triples iterates a [time, price, shipping, ...] series. A trailing
// incomplete group is ignored.
func (h History) Triples() iter.Seq2[Time, PriceShipping] {
	return func(yield func(Time, PriceShipping) bool) {
		for i := 0; i+2 < len(h); i += 3 {
			if !yield(Time(h[i]), PriceShipping{Price: h[i+1], Shipping: h[i+2]}) {
				return
			}
		}
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test -race -count=1 ./...`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git -C /Users/jameseagle/code/Jleagle/keepa add time.go time_test.go
git -C /Users/jameseagle/code/Jleagle/keepa commit -m "Add Time and History types"
```

---

### Task 3: CSVType and CSV

**Files:**
- Create: `csv.go`
- Test: `csv_test.go`

**Interfaces:**
- Consumes: `History` (Task 2).
- Produces: `type CSVType int` with constants `CSVAmazon` (0) through `CSVCountNewFBM` (35) and unexported `csvTypeCount`; `func (t CSVType) String() string`; `func (t CSVType) Valid() bool`; `func (t CSVType) HasShipping() bool`; `type CSV struct` with 36 `History` fields; `func (c *CSV) UnmarshalJSON([]byte) error`; `func (c *CSV) Get(t CSVType) History`.

- [ ] **Step 1: Write the failing tests**

`csv_test.go`:

```go
package keepa

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestCSVUnmarshalPositional(t *testing.T) {
	raw := `[[1,100,2,200],null,[3,300]]`
	var c CSV
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Amazon, History{1, 100, 2, 200}) {
		t.Errorf("Amazon = %v", c.Amazon)
	}
	if c.New != nil {
		t.Errorf("New = %v, want nil for a null entry", c.New)
	}
	if !slices.Equal(c.Used, History{3, 300}) {
		t.Errorf("Used = %v", c.Used)
	}
	if c.Sales != nil {
		t.Errorf("Sales = %v, want nil for an absent entry", c.Sales)
	}
	if got := c.Get(CSVUsed); !slices.Equal(got, History{3, 300}) {
		t.Errorf("Get(CSVUsed) = %v", got)
	}
	if got := c.Get(CSVType(99)); got != nil {
		t.Errorf("Get(99) = %v, want nil", got)
	}
	if got := c.Get(CSVType(-1)); got != nil {
		t.Errorf("Get(-1) = %v, want nil", got)
	}
}

func TestCSVUnmarshalAllIndices(t *testing.T) {
	series := make([][]int, int(csvTypeCount))
	for i := range series {
		series[i] = []int{i, i}
	}
	raw, err := json.Marshal(series)
	if err != nil {
		t.Fatal(err)
	}
	var c CSV
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	for i := CSVType(0); i < csvTypeCount; i++ {
		if got := c.Get(i); !slices.Equal(got, History{int(i), int(i)}) {
			t.Errorf("Get(%s) = %v, want [%d %d]", i, got, i, i)
		}
	}
	if !slices.Equal(c.Amazon, History{0, 0}) || !slices.Equal(c.CountNewFBM, History{35, 35}) {
		t.Errorf("first/last fields wrong: %v %v", c.Amazon, c.CountNewFBM)
	}
}

func TestCSVUnmarshalExtraSeriesIgnored(t *testing.T) {
	series := make([][]int, 40)
	raw, _ := json.Marshal(series)
	var c CSV
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("40 series should decode: %v", err)
	}
}

func TestCSVUnmarshalNull(t *testing.T) {
	c := CSV{Amazon: History{1, 2}}
	if err := json.Unmarshal([]byte(`null`), &c); err != nil {
		t.Fatal(err)
	}
	if c.Amazon != nil {
		t.Errorf("null should clear fields, Amazon = %v", c.Amazon)
	}
}

func TestCSVUnmarshalInvalid(t *testing.T) {
	var c CSV
	if err := json.Unmarshal([]byte(`{"a":1}`), &c); err == nil {
		t.Error("an object should not decode as CSV")
	}
}

func TestCSVTypeHasShipping(t *testing.T) {
	want := map[CSVType]bool{
		CSVNewFBMShipping: true, CSVBuyBoxShipping: true, CSVUsedNewShipping: true,
		CSVUsedVeryGoodShipping: true, CSVUsedGoodShipping: true, CSVUsedAcceptableShipping: true,
		CSVCollectibleNewShipping: true, CSVCollectibleVeryGoodShipping: true,
		CSVCollectibleGoodShipping: true, CSVCollectibleAcceptableShipping: true,
		CSVRefurbishedShipping: true, CSVEbayNewShipping: true, CSVEbayUsedShipping: true,
		CSVBuyBoxUsedShipping: true,
	}
	for i := CSVType(0); i < csvTypeCount; i++ {
		if got := i.HasShipping(); got != want[i] {
			t.Errorf("%s.HasShipping() = %v, want %v", i, got, want[i])
		}
	}
}

func TestCSVTypeStringAndValid(t *testing.T) {
	tests := map[CSVType]string{
		CSVAmazon:         "AMAZON",
		CSVNewFBMShipping: "NEW_FBM_SHIPPING",
		CSVPrimeExclusive: "PRIME_EXCL",
		CSVCountNewFBM:    "COUNT_NEW_FBM",
		CSVType(36):       "CSVType(36)",
		CSVType(-1):       "CSVType(-1)",
	}
	for typ, want := range tests {
		if got := typ.String(); got != want {
			t.Errorf("CSVType(%d).String() = %q, want %q", typ, got, want)
		}
	}
	if !CSVAmazon.Valid() || !CSVCountNewFBM.Valid() || CSVType(36).Valid() || CSVType(-1).Valid() {
		t.Error("Valid() boundaries wrong")
	}
	if int(csvTypeCount) != 36 {
		t.Errorf("csvTypeCount = %d, want 36", csvTypeCount)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... 2>&1 | head -20`
Expected: build failure mentioning `undefined: CSV`.

- [ ] **Step 3: Write the implementation**

`csv.go`:

```go
package keepa

import (
	"encoding/json"
	"fmt"
)

// CSVType indexes the price-type series in a product's csv field and in the
// per-price-type arrays of ProductStats and Deal.
type CSVType int

const (
	CSVAmazon CSVType = iota
	CSVNew
	CSVUsed
	CSVSales
	CSVListPrice
	CSVCollectible
	CSVRefurbished
	CSVNewFBMShipping
	CSVLightningDeal
	CSVWarehouse
	CSVNewFBA
	CSVCountNew
	CSVCountUsed
	CSVCountRefurbished
	CSVCountCollectible
	CSVExtraInfoUpdates
	CSVRating
	CSVCountReviews
	CSVBuyBoxShipping
	CSVUsedNewShipping
	CSVUsedVeryGoodShipping
	CSVUsedGoodShipping
	CSVUsedAcceptableShipping
	CSVCollectibleNewShipping
	CSVCollectibleVeryGoodShipping
	CSVCollectibleGoodShipping
	CSVCollectibleAcceptableShipping
	CSVRefurbishedShipping
	CSVEbayNewShipping
	CSVEbayUsedShipping
	CSVTradeIn
	CSVRental
	CSVBuyBoxUsedShipping
	CSVPrimeExclusive
	CSVCountNewFBA
	CSVCountNewFBM

	csvTypeCount // number of known series
)

var csvTypeNames = [csvTypeCount]string{
	"AMAZON", "NEW", "USED", "SALES", "LISTPRICE", "COLLECTIBLE", "REFURBISHED",
	"NEW_FBM_SHIPPING", "LIGHTNING_DEAL", "WAREHOUSE", "NEW_FBA", "COUNT_NEW",
	"COUNT_USED", "COUNT_REFURBISHED", "COUNT_COLLECTIBLE", "EXTRA_INFO_UPDATES",
	"RATING", "COUNT_REVIEWS", "BUY_BOX_SHIPPING", "USED_NEW_SHIPPING",
	"USED_VERY_GOOD_SHIPPING", "USED_GOOD_SHIPPING", "USED_ACCEPTABLE_SHIPPING",
	"COLLECTIBLE_NEW_SHIPPING", "COLLECTIBLE_VERY_GOOD_SHIPPING",
	"COLLECTIBLE_GOOD_SHIPPING", "COLLECTIBLE_ACCEPTABLE_SHIPPING",
	"REFURBISHED_SHIPPING", "EBAY_NEW_SHIPPING", "EBAY_USED_SHIPPING", "TRADE_IN",
	"RENT", "BUY_BOX_USED_SHIPPING", "PRIME_EXCL", "COUNT_NEW_FBA", "COUNT_NEW_FBM",
}

// Valid reports whether t is a series Keepa documents.
func (t CSVType) Valid() bool { return t >= 0 && t < csvTypeCount }

// String returns Keepa's name for the series, such as "BUY_BOX_SHIPPING".
func (t CSVType) String() string {
	if t.Valid() {
		return csvTypeNames[t]
	}
	return fmt.Sprintf("CSVType(%d)", int(t))
}

// HasShipping reports whether the series holds [time, price, shipping]
// triples rather than [time, value] pairs. Keepa documents this for types
// 7, 18 through 29 and 32.
func (t CSVType) HasShipping() bool {
	switch {
	case t == CSVNewFBMShipping, t == CSVBuyBoxUsedShipping:
		return true
	case t >= CSVBuyBoxShipping && t <= CSVEbayUsedShipping:
		return true
	}
	return false
}

// CSV holds a product's price and rank history, one series per CSVType.
// Keepa sends it as a positional array of arrays; fields are filled by index.
type CSV struct {
	Amazon                        History
	New                           History
	Used                          History
	Sales                         History
	ListPrice                     History
	Collectible                   History
	Refurbished                   History
	NewFBMShipping                History
	LightningDeal                 History
	Warehouse                     History
	NewFBA                        History
	CountNew                      History
	CountUsed                     History
	CountRefurbished              History
	CountCollectible              History
	ExtraInfoUpdates              History
	Rating                        History
	CountReviews                  History
	BuyBoxShipping                History
	UsedNewShipping               History
	UsedVeryGoodShipping          History
	UsedGoodShipping              History
	UsedAcceptableShipping        History
	CollectibleNewShipping        History
	CollectibleVeryGoodShipping   History
	CollectibleGoodShipping       History
	CollectibleAcceptableShipping History
	RefurbishedShipping           History
	EbayNewShipping               History
	EbayUsedShipping              History
	TradeIn                       History
	Rental                        History
	BuyBoxUsedShipping            History
	PrimeExclusive                History
	CountNewFBA                   History
	CountNewFBM                   History
}

func (c *CSV) fields() [csvTypeCount]*History {
	return [csvTypeCount]*History{
		&c.Amazon, &c.New, &c.Used, &c.Sales, &c.ListPrice, &c.Collectible,
		&c.Refurbished, &c.NewFBMShipping, &c.LightningDeal, &c.Warehouse,
		&c.NewFBA, &c.CountNew, &c.CountUsed, &c.CountRefurbished,
		&c.CountCollectible, &c.ExtraInfoUpdates, &c.Rating, &c.CountReviews,
		&c.BuyBoxShipping, &c.UsedNewShipping, &c.UsedVeryGoodShipping,
		&c.UsedGoodShipping, &c.UsedAcceptableShipping, &c.CollectibleNewShipping,
		&c.CollectibleVeryGoodShipping, &c.CollectibleGoodShipping,
		&c.CollectibleAcceptableShipping, &c.RefurbishedShipping,
		&c.EbayNewShipping, &c.EbayUsedShipping, &c.TradeIn, &c.Rental,
		&c.BuyBoxUsedShipping, &c.PrimeExclusive, &c.CountNewFBA, &c.CountNewFBM,
	}
}

// UnmarshalJSON decodes Keepa's positional array of series. A null entry
// leaves its field nil; series beyond the known range are ignored.
func (c *CSV) UnmarshalJSON(b []byte) error {
	var data []History
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	fields := c.fields()
	for i := range fields {
		if i < len(data) {
			*fields[i] = data[i]
		} else {
			*fields[i] = nil
		}
	}
	return nil
}

// Get returns the series for t, or nil when t is not valid.
func (c *CSV) Get(t CSVType) History {
	if !t.Valid() {
		return nil
	}
	return *c.fields()[t]
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test -race -count=1 ./...`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git -C /Users/jameseagle/code/Jleagle/keepa add csv.go csv_test.go
git -C /Users/jameseagle/code/Jleagle/keepa commit -m "Add CSVType enum and positional CSV decoder"
```

---

### Task 4: Errors

**Files:**
- Create: `errors.go`
- Test: `errors_test.go`

**Interfaces:**
- Produces: `ErrInvalidRequest`, `ErrNotEnoughTokens`, `ErrWouldWait`; `type APIError struct{ StatusCode int; Type, Message, Details string }` with `Error()` and `Is()`; `type HTTPError struct{ StatusCode int; Body []byte }` with `Error()`; `type TokenWaitError struct{ Wait time.Duration; Cost, Reserve int; Projected float64 }` with `Error()` and `Is()`; unexported `func invalidRequest(format string, args ...any) error`.

- [ ] **Step 1: Write the failing tests**

`errors_test.go`:

```go
package keepa

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAPIErrorIsNotEnoughTokens(t *testing.T) {
	err := error(&APIError{StatusCode: http.StatusTooManyRequests, Type: "notEnoughTokens", Message: "no tokens"})
	if !errors.Is(err, ErrNotEnoughTokens) {
		t.Error("429 APIError should match ErrNotEnoughTokens")
	}
	if errors.Is(&APIError{StatusCode: 400, Type: "invalidParameter"}, ErrNotEnoughTokens) {
		t.Error("400 APIError should not match ErrNotEnoughTokens")
	}
	if errors.Is(err, ErrInvalidRequest) {
		t.Error("APIError should not match ErrInvalidRequest")
	}
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok || apiErr.Type != "notEnoughTokens" {
		t.Errorf("errors.AsType failed: %v %v", apiErr, ok)
	}
}

func TestAPIErrorString(t *testing.T) {
	err := &APIError{StatusCode: 400, Type: "invalidParameter", Message: "bad key", Details: "see docs"}
	got := err.Error()
	for _, want := range []string{"keepa:", "invalidParameter", "400", "bad key", "see docs"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, missing %q", got, want)
		}
	}
	plain := (&APIError{StatusCode: 400, Type: "x", Message: "m"}).Error()
	if strings.Contains(plain, "()") {
		t.Errorf("empty details should not print parentheses: %q", plain)
	}
}

func TestHTTPErrorString(t *testing.T) {
	err := &HTTPError{StatusCode: 502, Body: []byte("gateway blew up")}
	got := err.Error()
	if !strings.HasPrefix(got, "keepa: HTTP 502") || !strings.Contains(got, "gateway blew up") {
		t.Errorf("Error() = %q", got)
	}
	long := &HTTPError{StatusCode: 500, Body: []byte(strings.Repeat("x", 500))}
	if len(long.Error()) > 260 {
		t.Errorf("long body not truncated: %d chars", len(long.Error()))
	}
}

func TestTokenWaitErrorIsWouldWait(t *testing.T) {
	err := error(&TokenWaitError{Wait: 90 * time.Second, Cost: 50, Reserve: 1000, Projected: 980})
	if !errors.Is(err, ErrWouldWait) {
		t.Error("TokenWaitError should match ErrWouldWait")
	}
	got := err.Error()
	for _, want := range []string{"keepa:", "50", "1000", "1m30s"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, missing %q", got, want)
		}
	}
}

func TestInvalidRequest(t *testing.T) {
	err := invalidRequest("between 1 and %d ASINs required, got %d", 100, 0)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Error("invalidRequest should wrap ErrInvalidRequest")
	}
	if got := err.Error(); got != "keepa: invalid request: between 1 and 100 ASINs required, got 0" {
		t.Errorf("Error() = %q", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... 2>&1 | head -20`
Expected: build failure mentioning `undefined: APIError`.

- [ ] **Step 3: Write the implementation**

`errors.go`:

```go
package keepa

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

var (
	// ErrInvalidRequest wraps every argument validation failure.
	ErrInvalidRequest = errors.New("keepa: invalid request")
	// ErrNotEnoughTokens matches an *APIError carried by an HTTP 429 response.
	ErrNotEnoughTokens = errors.New("keepa: not enough tokens")
	// ErrWouldWait matches a *TokenWaitError from a call made WithoutWaiting.
	ErrWouldWait = errors.New("keepa: call would wait for tokens")
)

// APIError is the error object Keepa includes in a response envelope.
type APIError struct {
	StatusCode int    `json:"-"` // HTTP status of the response that carried it
	Type       string `json:"type"`
	Message    string `json:"message"`
	Details    string `json:"details"`
}

func (e *APIError) Error() string {
	s := fmt.Sprintf("keepa: api error %s (HTTP %d): %s", e.Type, e.StatusCode, e.Message)
	if e.Details != "" {
		s += " (" + e.Details + ")"
	}
	return s
}

// Is reports ErrNotEnoughTokens for a 429 response.
func (e *APIError) Is(target error) bool {
	return target == ErrNotEnoughTokens && e.StatusCode == http.StatusTooManyRequests
}

// HTTPError is a non-200 response whose body was not a Keepa envelope.
type HTTPError struct {
	StatusCode int
	Body       []byte
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("keepa: HTTP %d: %s", e.StatusCode, truncate(e.Body, 200))
}

// TokenWaitError is returned by a call made WithoutWaiting when the bucket
// cannot pay for it without dropping below the reserve. Wait is how long the
// client would have slept.
type TokenWaitError struct {
	Wait      time.Duration
	Cost      int
	Reserve   int
	Projected float64
}

func (e *TokenWaitError) Error() string {
	return fmt.Sprintf("keepa: call costing %d tokens would wait %s to keep %d in reserve (%.0f projected)",
		e.Cost, e.Wait.Round(time.Second), e.Reserve, e.Projected)
}

// Is reports ErrWouldWait.
func (e *TokenWaitError) Is(target error) bool { return target == ErrWouldWait }

func invalidRequest(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRequest, fmt.Sprintf(format, args...))
}

// truncate returns b as a string, cut to n bytes with an ellipsis.
func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test -race -count=1 ./...`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git -C /Users/jameseagle/code/Jleagle/keepa add errors.go errors_test.go
git -C /Users/jameseagle/code/Jleagle/keepa commit -m "Add error types and sentinels"
```

---

### Task 5: Client, option plumbing and token bucket

**Files:**
- Create: `keepa.go`, `options.go`, `tokens.go`
- Test: `keepa_test.go`, `options_test.go`, `tokens_test.go`

**Interfaces:**
- Consumes: `TokenWaitError` (Task 4).
- Produces: `Client`, `Option`, `NewClient(apiKey string, opts ...Option) *Client`, `Limiter`, client options `WithHTTPClient`, `WithBaseURL`, `WithLimiter`, `WithLogger`, `WithTokenCallback`, `WithTokenReserve`, `WithTimeout`; `TokenState` with `NetRefillRate()` and `Projected(time.Time)`; `TokenUpdate`; `func (c *Client) Tokens() TokenState`; unexported `callParams`, all seven `*Params` structs, the seven `*Option` interfaces, `CallOption`, `WithReserve`, `WithoutWaiting`; unexported `reserveTokens`, `refundTokens`, `waitForTokens`; unexported client fields `apiKey, baseURL, httpClient, limiter, logger, onTokens, reserve, timeout, now, tokens`.

- [ ] **Step 1: Write the failing tests**

`keepa_test.go`:

```go
package keepa

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

type fakeLimiter struct {
	calls int
	err   error
}

func (f *fakeLimiter) Wait(ctx context.Context) error {
	f.calls++
	return f.err
}

// captureLogs returns a logger whose output can be inspected.
func captureLogs() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestNewClientDefaults(t *testing.T) {
	c := NewClient("key")
	if c.apiKey != "key" {
		t.Errorf("apiKey = %q", c.apiKey)
	}
	if c.baseURL != "https://api.keepa.com" {
		t.Errorf("baseURL = %q", c.baseURL)
	}
	if c.reserve != 20 {
		t.Errorf("reserve = %d, want 20", c.reserve)
	}
	if c.timeout != time.Minute {
		t.Errorf("timeout = %v, want 1m", c.timeout)
	}
	if c.httpClient == nil || c.logger == nil || c.now == nil {
		t.Error("httpClient, logger and now must have defaults")
	}
	if c.limiter != nil || c.onTokens != nil {
		t.Error("limiter and callback default to nil")
	}
	if c.Tokens().Known {
		t.Error("a new client must not claim to know the bucket")
	}
}

func TestNewClientOptions(t *testing.T) {
	hc := &http.Client{}
	logger, _ := captureLogs()
	lim := &fakeLimiter{}
	called := false
	c := NewClient("key",
		WithHTTPClient(hc),
		WithBaseURL("http://example.test/"),
		WithLimiter(lim),
		WithLogger(logger),
		WithTokenCallback(func(TokenUpdate) { called = true }),
		WithTokenReserve(1000),
		WithTimeout(5*time.Second),
	)
	if c.httpClient != hc {
		t.Error("WithHTTPClient not applied")
	}
	if c.baseURL != "http://example.test" {
		t.Errorf("WithBaseURL should trim the trailing slash: %q", c.baseURL)
	}
	if c.limiter != lim {
		t.Error("WithLimiter not applied")
	}
	if c.logger != logger {
		t.Error("WithLogger not applied")
	}
	if c.reserve != 1000 {
		t.Errorf("reserve = %d, want 1000", c.reserve)
	}
	if c.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", c.timeout)
	}
	c.onTokens(TokenUpdate{})
	if !called {
		t.Error("WithTokenCallback not applied")
	}
}
```

`options_test.go`:

```go
package keepa

import "testing"

func TestCallOptionsApplyToEveryEndpoint(t *testing.T) {
	endpoints := []struct {
		name  string
		apply func(CallOption) callParams
	}{
		{"product", func(o CallOption) callParams { var p productParams; o.applyProduct(&p); return p.callParams }},
		{"bestsellers", func(o CallOption) callParams { var p bestSellersParams; o.applyBestSellers(&p); return p.callParams }},
		{"category", func(o CallOption) callParams { var p categoryParams; o.applyCategory(&p); return p.callParams }},
		{"deals", func(o CallOption) callParams { var p dealsParams; o.applyDeals(&p); return p.callParams }},
		{"lightning", func(o CallOption) callParams { var p lightningDealsParams; o.applyLightningDeals(&p); return p.callParams }},
		{"search", func(o CallOption) callParams { var p searchParams; o.applySearch(&p); return p.callParams }},
		{"sellers", func(o CallOption) callParams { var p sellersParams; o.applySellers(&p); return p.callParams }},
	}
	for _, e := range endpoints {
		got := e.apply(WithReserve(5))
		if got.reserve == nil || *got.reserve != 5 {
			t.Errorf("%s: WithReserve(5) not applied: %+v", e.name, got)
		}
		if got.noWait {
			t.Errorf("%s: WithReserve must not set noWait", e.name)
		}
		got = e.apply(WithoutWaiting())
		if !got.noWait {
			t.Errorf("%s: WithoutWaiting not applied", e.name)
		}
		if got.reserve != nil {
			t.Errorf("%s: WithoutWaiting must not set reserve", e.name)
		}
	}
}
```

`tokens_test.go`:

```go
package keepa

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// testNow pins the client clock in every test that needs determinism.
var testNow = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

func bucketClient(state TokenState, opts ...Option) *Client {
	c := NewClient("key", opts...)
	c.now = func() time.Time { return testNow }
	c.tokens.state = state
	return c
}

func TestTokenStateProjected(t *testing.T) {
	s := TokenState{Known: true, Left: 100, RefillRate: 10, FlowReduction: 2.5, UpdatedAt: testNow}
	if got := s.NetRefillRate(); got != 7.5 {
		t.Errorf("NetRefillRate() = %v, want 7.5", got)
	}
	if got := s.Projected(testNow.Add(4 * time.Minute)); got != 130 {
		t.Errorf("Projected(+4m) = %v, want 130", got)
	}
	if got := s.Projected(testNow.Add(-2 * time.Minute)); got != 85 {
		t.Errorf("Projected(-2m) = %v, want 85", got)
	}
	if got := (TokenState{}).Projected(testNow); got != 0 {
		t.Errorf("unknown state Projected() = %v, want 0", got)
	}
	if got := (TokenState{RefillRate: 3, FlowReduction: 5}).NetRefillRate(); got != 1 {
		t.Errorf("NetRefillRate() floor = %v, want 1", got)
	}
}

func TestReserveTokensRunsWhenPlentiful(t *testing.T) {
	c := bucketClient(TokenState{Known: true, Left: 100, RefillRate: 10, UpdatedAt: testNow})
	wait, err := c.reserveTokens(30, 20, false)
	if err != nil || wait != 0 {
		t.Fatalf("reserveTokens = %v, %v; want 0, nil", wait, err)
	}
	if s := c.Tokens(); s.Left != 70 || !s.UpdatedAt.Equal(testNow) {
		t.Errorf("state after = %+v, want Left 70 at testNow", s)
	}
}

func TestReserveTokensWaitsToKeepReserve(t *testing.T) {
	// 30 in hand, need 30 + 20 reserve = 50, refill 10/min: two minutes.
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, UpdatedAt: testNow})
	wait, err := c.reserveTokens(30, 20, false)
	if err != nil || wait != 2*time.Minute {
		t.Fatalf("reserveTokens = %v, %v; want 2m, nil", wait, err)
	}
	if s := c.Tokens(); s.Left != 20 || !s.UpdatedAt.Equal(testNow.Add(2*time.Minute)) {
		t.Errorf("state after = %+v, want Left 20 at testNow+2m", s)
	}
}

func TestReserveTokensProjectsRefill(t *testing.T) {
	// Seen empty three minutes ago at 10/min: 30 projected, which covers cost 10 + reserve 20 exactly.
	c := bucketClient(TokenState{Known: true, Left: 0, RefillRate: 10, UpdatedAt: testNow.Add(-3 * time.Minute)})
	wait, err := c.reserveTokens(10, 20, false)
	if err != nil || wait != 0 {
		t.Fatalf("reserveTokens = %v, %v; want 0, nil", wait, err)
	}
	if s := c.Tokens(); s.Left != 20 || !s.UpdatedAt.Equal(testNow) {
		t.Errorf("state after = %+v, want Left 20 at testNow", s)
	}
}

func TestReserveTokensUsesNetRefillRate(t *testing.T) {
	// 10/min minus 4/min tracking: 6/min. 20 short at 6/min is 200 seconds.
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, FlowReduction: 4, UpdatedAt: testNow})
	wait, _ := c.reserveTokens(30, 20, false)
	if wait != 200*time.Second {
		t.Errorf("wait = %v, want 3m20s", wait)
	}
}

func TestReserveTokensQueuesConcurrentCallers(t *testing.T) {
	// At the floor with 20 in hand and 60/min refill, two calls of 60 each:
	// the first waits one minute, the second queues behind it and waits two.
	c := bucketClient(TokenState{Known: true, Left: 20, RefillRate: 60, UpdatedAt: testNow})
	w1, _ := c.reserveTokens(60, 20, false)
	w2, _ := c.reserveTokens(60, 20, false)
	if w1 != time.Minute || w2 != 2*time.Minute {
		t.Errorf("waits = %v, %v; want 1m, 2m", w1, w2)
	}
	if s := c.Tokens(); !s.UpdatedAt.Equal(testNow.Add(2 * time.Minute)) {
		t.Errorf("timeline = %v, want testNow+2m", s.UpdatedAt)
	}
}

func TestReserveTokensWithoutWaiting(t *testing.T) {
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, UpdatedAt: testNow})
	_, err := c.reserveTokens(30, 20, true)
	werr, ok := errors.AsType[*TokenWaitError](err)
	if !ok {
		t.Fatalf("expected *TokenWaitError, got %T: %v", err, err)
	}
	if werr.Wait != 2*time.Minute || werr.Cost != 30 || werr.Reserve != 20 || werr.Projected != 30 {
		t.Errorf("TokenWaitError = %+v", werr)
	}
	if !errors.Is(err, ErrWouldWait) {
		t.Error("should match ErrWouldWait")
	}
	if s := c.Tokens(); s.Left != 30 || !s.UpdatedAt.Equal(testNow) {
		t.Errorf("non-blocking mode must not reserve: %+v", s)
	}
}

func TestReserveTokensUnknownStateOrZeroCost(t *testing.T) {
	c := bucketClient(TokenState{})
	if wait, err := c.reserveTokens(500, 1000, true); wait != 0 || err != nil {
		t.Errorf("unknown state must run immediately: %v %v", wait, err)
	}
	c = bucketClient(TokenState{Known: true, Left: 0, RefillRate: 10, UpdatedAt: testNow})
	if wait, err := c.reserveTokens(0, 1000, true); wait != 0 || err != nil {
		t.Errorf("cost 0 must run immediately: %v %v", wait, err)
	}
	if s := c.Tokens(); s.Left != 0 {
		t.Errorf("cost 0 must not change state: %+v", s)
	}
}

func TestRefundTokensRestoresProjection(t *testing.T) {
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, UpdatedAt: testNow})
	before := c.Tokens().Projected(testNow)
	if _, err := c.reserveTokens(30, 20, false); err != nil {
		t.Fatal(err)
	}
	c.refundTokens(30, testNow)
	if after := c.Tokens().Projected(testNow); after != before {
		t.Errorf("projection after refund = %v, want %v", after, before)
	}
}

func TestRefundTokensSkipsWhenNothingReserved(t *testing.T) {
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, UpdatedAt: testNow})
	c.refundTokens(30, testNow)
	if s := c.Tokens(); !s.UpdatedAt.Equal(testNow) {
		t.Errorf("refund moved a timeline that was not in the future: %v", s.UpdatedAt)
	}
}

func TestWaitForTokensSleepsThenRuns(t *testing.T) {
	// Two tokens short at 6000/min is 20ms.
	logger, logs := captureLogs()
	c := bucketClient(TokenState{Known: true, Left: 19, RefillRate: 6000, UpdatedAt: testNow}, WithLogger(logger))
	start := time.Now()
	if err := c.waitForTokens(t.Context(), 1, 20, false); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
		t.Errorf("returned after %v, expected a wait of about 20ms", elapsed)
	}
	if !strings.Contains(logs.String(), "keepa: waiting for tokens") {
		t.Errorf("wait not logged: %s", logs.String())
	}
}

func TestWaitForTokensCancelRefunds(t *testing.T) {
	// Empty bucket at 1/min: a cost of 1 with reserve 20 would wait 21 minutes.
	c := bucketClient(TokenState{Known: true, Left: 0, RefillRate: 1, UpdatedAt: testNow})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := c.waitForTokens(ctx, 1, 20, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if got := c.Tokens().Projected(testNow); got != 0 {
		t.Errorf("projection after cancel = %v, want 0 (slot refunded)", got)
	}
}

func TestWaitForTokensNoWaitDoesNotLog(t *testing.T) {
	logger, logs := captureLogs()
	c := bucketClient(TokenState{Known: true, Left: 0, RefillRate: 1, UpdatedAt: testNow}, WithLogger(logger))
	if err := c.waitForTokens(t.Context(), 1, 20, true); !errors.Is(err, ErrWouldWait) {
		t.Fatalf("err = %v", err)
	}
	if logs.Len() != 0 {
		t.Errorf("non-blocking mode logged: %s", logs.String())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./... 2>&1 | head -20`
Expected: build failure mentioning `undefined: NewClient`.

- [ ] **Step 3: Write the implementation**

`keepa.go`:

```go
package keepa

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL      = "https://api.keepa.com"
	defaultTimeout      = time.Minute
	defaultTokenReserve = 20
)

// Limiter paces requests before the token wait. *rate.Limiter from
// golang.org/x/time/rate satisfies it.
type Limiter interface {
	Wait(ctx context.Context) error
}

// Client calls the Keepa API. It is safe for concurrent use.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	limiter    Limiter
	logger     *slog.Logger
	onTokens   func(TokenUpdate)
	reserve    int
	timeout    time.Duration
	now        func() time.Time

	tokens tokenBucket
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets the transport. The default has a 30 second timeout.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// WithBaseURL points the client at another server, for example a test server.
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

// WithLimiter waits on l before every request, ahead of the token wait.
func WithLimiter(l Limiter) Option { return func(c *Client) { c.limiter = l } }

// WithLogger receives the client's diagnostics. The default discards them.
func WithLogger(l *slog.Logger) Option { return func(c *Client) { c.logger = l } }

// WithTokenCallback is invoked for every Keepa envelope received, including
// error envelopes, with the token counts it carried.
func WithTokenCallback(fn func(TokenUpdate)) Option { return func(c *Client) { c.onTokens = fn } }

// WithTokenReserve sets the number of tokens the client keeps in hand. A call
// that would drop the bucket below it waits until the refill covers it.
// Individual calls override it with WithReserve. The default is 20.
func WithTokenReserve(n int) Option { return func(c *Client) { c.reserve = n } }

// WithTimeout sets the deadline applied to a request whose context has none.
// The default is one minute; best sellers uses double.
func WithTimeout(d time.Duration) Option { return func(c *Client) { c.timeout = d } }

// NewClient returns a client for the given API key.
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     slog.New(slog.DiscardHandler),
		reserve:    defaultTokenReserve,
		timeout:    defaultTimeout,
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}
```

`options.go`:

```go
package keepa

import "time"

// callParams is the per-call policy every endpoint accepts.
type callParams struct {
	reserve *int // overrides Client.reserve when set
	noWait  bool // return *TokenWaitError instead of sleeping
}

// Per-endpoint parameter sets. The endpoint files define the option
// constructors that populate them.

type productParams struct {
	callParams
	stats      *time.Time
	ratings    bool
	liveUpdate bool
	buyBox     bool
	videos     bool
	noHistory  bool
}

type bestSellersParams struct {
	callParams
	rankRange  int
	variations bool
}

type categoryParams struct {
	callParams
	parents bool
}

type dealsParams struct {
	callParams
	page         int
	dateRange    *int
	minRating    *int
	outOfStock   bool
	filterErotic bool
	sort         *int
	deltaLast    *[2]int
}

type lightningDealsParams struct {
	callParams
}

type searchParams struct {
	callParams
	asinsOnly bool
}

type sellersParams struct {
	callParams
}

// ProductOption configures GetProducts.
type ProductOption interface{ applyProduct(*productParams) }

// BestSellersOption configures GetBestSellers.
type BestSellersOption interface{ applyBestSellers(*bestSellersParams) }

// CategoryOption configures GetCategories.
type CategoryOption interface{ applyCategory(*categoryParams) }

// DealsOption configures GetDeals.
type DealsOption interface{ applyDeals(*dealsParams) }

// LightningDealsOption configures GetLightningDeals.
type LightningDealsOption interface{ applyLightningDeals(*lightningDealsParams) }

// SearchOption configures SearchProducts.
type SearchOption interface{ applySearch(*searchParams) }

// SellersOption configures GetSellers.
type SellersOption interface{ applySellers(*sellersParams) }

type productOption func(*productParams)
type bestSellersOption func(*bestSellersParams)
type categoryOption func(*categoryParams)
type dealsOption func(*dealsParams)
type searchOption func(*searchParams)

func (o productOption) applyProduct(p *productParams)             { o(p) }
func (o bestSellersOption) applyBestSellers(p *bestSellersParams) { o(p) }
func (o categoryOption) applyCategory(p *categoryParams)          { o(p) }
func (o dealsOption) applyDeals(p *dealsParams)                   { o(p) }
func (o searchOption) applySearch(p *searchParams)                { o(p) }

// CallOption is accepted by every endpoint method.
type CallOption interface {
	ProductOption
	BestSellersOption
	CategoryOption
	DealsOption
	LightningDealsOption
	SearchOption
	SellersOption
}

type callOption func(*callParams)

func (o callOption) applyProduct(p *productParams)               { o(&p.callParams) }
func (o callOption) applyBestSellers(p *bestSellersParams)       { o(&p.callParams) }
func (o callOption) applyCategory(p *categoryParams)             { o(&p.callParams) }
func (o callOption) applyDeals(p *dealsParams)                   { o(&p.callParams) }
func (o callOption) applyLightningDeals(p *lightningDealsParams) { o(&p.callParams) }
func (o callOption) applySearch(p *searchParams)                 { o(&p.callParams) }
func (o callOption) applySellers(p *sellersParams)               { o(&p.callParams) }

// WithReserve overrides the client's token reserve for one call. Background
// work keeps the client floor; an interactive call can pass WithReserve(0)
// to run as long as the bucket can pay for it.
func WithReserve(n int) CallOption {
	return callOption(func(p *callParams) { p.reserve = &n })
}

// WithoutWaiting makes the call return a *TokenWaitError, matching
// ErrWouldWait, instead of sleeping when the bucket cannot pay for it while
// keeping the reserve. No slot is reserved.
func WithoutWaiting() CallOption {
	return callOption(func(p *callParams) { p.noWait = true })
}
```

`tokens.go`:

```go
package keepa

import (
	"context"
	"sync"
	"time"
)

// TokenState is a snapshot of the client's view of the Keepa token bucket.
type TokenState struct {
	Known         bool      // false until an envelope or seed has been recorded
	Left          int       // tokens in the bucket at UpdatedAt
	RefillRate    int       // tokens per minute granted by the plan
	FlowReduction float64   // tokens per minute lost to tracking subscriptions
	UpdatedAt     time.Time // when Left was true; in the future while calls are queued
}

// NetRefillRate is the effective refill in tokens per minute, never below 1.
func (s TokenState) NetRefillRate() float64 {
	net := float64(s.RefillRate) - s.FlowReduction
	if net <= 0 {
		return 1
	}
	return net
}

// Projected estimates the tokens available at the given time. Times before
// UpdatedAt project downwards, which is how queued calls reserve their slots.
func (s TokenState) Projected(at time.Time) float64 {
	if !s.Known {
		return 0
	}
	return float64(s.Left) + at.Sub(s.UpdatedAt).Minutes()*s.NetRefillRate()
}

// TokenUpdate is passed to the callback registered with WithTokenCallback.
type TokenUpdate struct {
	Left          int
	Consumed      int
	RefillRate    int
	RefillIn      time.Duration
	FlowReduction float64
	Path          string    // request path, such as "/product"
	Timestamp     time.Time // server time from the envelope
}

type tokenBucket struct {
	mu      sync.Mutex
	state   TokenState
	seeding bool
}

// Tokens returns a snapshot of the client's view of the token bucket.
func (c *Client) Tokens() TokenState {
	c.tokens.mu.Lock()
	defer c.tokens.mu.Unlock()
	return c.tokens.state
}

// reserveTokens applies the wait rule. It returns how long the caller must
// wait before running (0 to run now) and, with noWait, a *TokenWaitError
// instead of reserving a slot.
func (c *Client) reserveTokens(cost, reserve int, noWait bool) (time.Duration, error) {
	c.tokens.mu.Lock()
	defer c.tokens.mu.Unlock()

	s := &c.tokens.state
	if !s.Known || cost <= 0 {
		return 0, nil
	}
	now := c.now()
	projected := s.Projected(now)
	target := float64(cost + reserve)
	if projected >= target {
		s.Left = int(projected) - cost
		s.UpdatedAt = now
		return 0, nil
	}
	wait := time.Duration((target - projected) / s.NetRefillRate() * float64(time.Minute))
	if noWait {
		return wait, &TokenWaitError{Wait: wait, Cost: cost, Reserve: reserve, Projected: projected}
	}
	s.Left = reserve
	s.UpdatedAt = now.Add(wait)
	return wait, nil
}

// refundTokens releases a slot whose call was cancelled before it ran.
func (c *Client) refundTokens(cost int, reservedAt time.Time) {
	c.tokens.mu.Lock()
	defer c.tokens.mu.Unlock()

	s := &c.tokens.state
	if s.UpdatedAt.After(reservedAt) {
		refund := time.Duration(float64(cost) / s.NetRefillRate() * float64(time.Minute))
		s.UpdatedAt = s.UpdatedAt.Add(-refund)
	}
}

// waitForTokens blocks until the bucket can pay cost while keeping reserve.
func (c *Client) waitForTokens(ctx context.Context, cost, reserve int, noWait bool) error {
	reservedAt := c.now()
	wait, err := c.reserveTokens(cost, reserve, noWait)
	if err != nil || wait <= 0 {
		return err
	}
	c.logger.Info("keepa: waiting for tokens", "cost", cost, "reserve", reserve, "wait", wait)

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		c.refundTokens(cost, reservedAt)
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test -race -count=1 ./...`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git -C /Users/jameseagle/code/Jleagle/keepa add keepa.go options.go tokens.go keepa_test.go options_test.go tokens_test.go
git -C /Users/jameseagle/code/Jleagle/keepa commit -m "Add client, option plumbing and token bucket"
```

---

### Task 6: Transport

**Files:**
- Create: `transport.go`, `testhelpers_test.go`
- Modify: `tokens.go` (add `recordEnvelope`)
- Test: `transport_test.go`

**Interfaces:**
- Consumes: `Client`, `callParams`, `waitForTokens`, `APIError`, `HTTPError`, `truncate`.
- Produces: `Envelope`; unexported `request{path, query, body, cost, timeout, callParams}`; `func (c *Client) query() url.Values`; `func do[T any](ctx context.Context, c *Client, r request) (*T, error)`; `func (c *Client) recordEnvelope(env *Envelope, path string)`; test helpers `newTestClient`, `recorder` (`Calls`, `Paths`, `Last`, `SetTokenHandler`), `okEnvelope`, `tokenFixture`, `serveJSON`, `serveFixture`, `probe`.

- [ ] **Step 1: Write the test helpers**

`testhelpers_test.go`:

```go
package keepa

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

const envelopeFields = `"timestamp":1790277678673,"tokensLeft":1195,"tokensConsumed":1,"refillIn":30000,"refillRate":20,"tokenFlowReduction":0,"processingTimeInMs":12`

// tokenFixture answers GET /token: 1200 tokens in hand, 20 per minute.
const tokenFixture = `{"timestamp":1790277678673,"tokensLeft":1200,"tokensConsumed":0,"refillIn":30000,"refillRate":20,"tokenFlowReduction":0,"processingTimeInMs":1}`

// okEnvelope wraps a payload fragment such as `"products":[]` in a 200 envelope
// that reports one token consumed and 1195 left.
func okEnvelope(payload string) string {
	if payload == "" {
		return "{" + envelopeFields + "}"
	}
	return "{" + envelopeFields + "," + payload + "}"
}

type recorded struct {
	method      string
	path        string
	query       url.Values
	contentType string
	body        []byte
}

type recorder struct {
	mu           sync.Mutex
	calls        []recorded
	tokenHandler http.HandlerFunc
}

func (r *recorder) Calls() []recorded {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

// Paths returns every request path in order, including the token seed.
func (r *recorder) Paths() []string {
	var paths []string
	for _, c := range r.Calls() {
		paths = append(paths, c.path)
	}
	return paths
}

// Last returns the most recent request that was not the token seed.
func (r *recorder) Last(t *testing.T) recorded {
	t.Helper()
	calls := r.Calls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].path != "/token" {
			return calls[i]
		}
	}
	t.Fatal("no request reached the endpoint")
	return recorded{}
}

// SetTokenHandler replaces the default /token response.
func (r *recorder) SetTokenHandler(h http.HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokenHandler = h
}

// newTestClient starts a server that records every request, answers GET /token
// with tokenFixture and everything else with handler, and returns a client
// pointed at it with the clock pinned to testNow.
func newTestClient(t *testing.T, handler http.HandlerFunc, opts ...Option) (*Client, *recorder) {
	t.Helper()
	rec := &recorder{tokenHandler: serveJSON(http.StatusOK, tokenFixture)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec.mu.Lock()
		rec.calls = append(rec.calls, recorded{
			method:      r.Method,
			path:        r.URL.Path,
			query:       r.URL.Query(),
			contentType: r.Header.Get("Content-Type"),
			body:        body,
		})
		tokenHandler := rec.tokenHandler
		rec.mu.Unlock()
		r.Body = io.NopCloser(bytes.NewReader(body))
		if r.URL.Path == "/token" {
			tokenHandler(w, r)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	c := NewClient("test-key", append([]Option{WithBaseURL(srv.URL)}, opts...)...)
	c.now = func() time.Time { return testNow }
	return c, rec
}

// serveJSON answers every request with status and body.
func serveJSON(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// serveFixture answers every request with the contents of testdata/<name>.
func serveFixture(t *testing.T, name string) http.HandlerFunc {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return serveJSON(http.StatusOK, string(body))
}
```

- [ ] **Step 2: Write the failing tests**

`transport_test.go`:

```go
package keepa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type probeResponse struct {
	Envelope
	Value string `json:"value"`
}

// probe runs do against a fake "/probe" endpoint.
func probe(ctx context.Context, c *Client, r request) (*probeResponse, error) {
	if r.path == "" {
		r.path = "/probe"
	}
	if r.query == nil {
		r.query = c.query()
	}
	return do[probeResponse](ctx, c, r)
}

// invalidKeyEnvelope is the real body Keepa returns for a bad key: an
// envelope with an error and all token fields zero.
const invalidKeyEnvelope = `{"error":{"details":"","message":"You used an invalid parameter for this API call.","type":"invalidParameter"},"processingTimeInMs":0,"refillIn":0,"refillRate":0,"timestamp":1790277678673,"tokenFlowReduction":0.0,"tokensConsumed":0,"tokensLeft":0}`

func TestDoSendsGetWithKeyAndDecodes(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope(`"value":"hi"`)))
	res, err := probe(t.Context(), c, request{cost: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Value != "hi" || res.TokensLeft != 1195 || res.RefillRate != 20 {
		t.Errorf("decoded %+v", res)
	}
	last := rec.Last(t)
	if last.method != http.MethodGet || last.path != "/probe" || last.query.Get("key") != "test-key" {
		t.Errorf("request = %+v", last)
	}
	if s := c.Tokens(); !s.Known || s.Left != 1195 || s.RefillRate != 20 || !s.UpdatedAt.Equal(testNow) {
		t.Errorf("bucket after = %+v", s)
	}
}

func TestDoPostsJSONBody(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	if _, err := probe(t.Context(), c, request{cost: 1, body: map[string]any{"page": 2}}); err != nil {
		t.Fatal(err)
	}
	last := rec.Last(t)
	if last.method != http.MethodPost || last.contentType != "application/json" {
		t.Errorf("request = %+v", last)
	}
	var body map[string]any
	if err := json.Unmarshal(last.body, &body); err != nil || body["page"] != float64(2) {
		t.Errorf("body = %s (%v)", last.body, err)
	}
	if last.query.Get("key") != "test-key" {
		t.Error("POST must still carry the key in the query")
	}
}

func TestDoRecordsTokensFromErrorEnvelope(t *testing.T) {
	// A quota-exhausted 429 still carries the real token counts. If they are
	// not recorded, schedulers keep working from the last successful reading.
	body := `{"timestamp":1786000000000,"tokensLeft":0,"tokensConsumed":0,"refillIn":60000,"refillRate":5,"tokenFlowReduction":0,"error":{"type":"notEnoughTokens","message":"You do not have enough tokens","details":""}}`
	var updates []TokenUpdate
	c, _ := newTestClient(t, serveJSON(http.StatusTooManyRequests, body), WithTokenCallback(func(u TokenUpdate) {
		if u.Path != "/token" {
			updates = append(updates, u)
		}
	}))
	_, err := probe(t.Context(), c, request{cost: 1})
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 429 || apiErr.Type != "notEnoughTokens" || apiErr.Message == "" {
		t.Errorf("APIError = %+v", apiErr)
	}
	if !errors.Is(err, ErrNotEnoughTokens) {
		t.Error("should match ErrNotEnoughTokens")
	}
	if len(updates) != 1 || updates[0].Left != 0 || updates[0].RefillRate != 5 || updates[0].Path != "/probe" {
		t.Errorf("updates = %+v", updates)
	}
	if s := c.Tokens(); !s.Known || s.Left != 0 || s.RefillRate != 5 {
		t.Errorf("bucket not updated from the error envelope: %+v", s)
	}
}

func TestDoIgnoresEnvelopeWithoutRefillRate(t *testing.T) {
	// Keepa answers a bad key with an envelope whose token fields are all
	// zero. That is not a bucket reading and must not be recorded.
	called := false
	c, _ := newTestClient(t, serveJSON(http.StatusBadRequest, invalidKeyEnvelope), WithTokenCallback(func(u TokenUpdate) {
		if u.Path != "/token" {
			called = true
		}
	}))
	c.tokens.state = TokenState{Known: true, Left: 1200, RefillRate: 20, UpdatedAt: testNow}
	_, err := probe(t.Context(), c, request{cost: 1})
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok || apiErr.Type != "invalidParameter" || apiErr.StatusCode != 400 {
		t.Fatalf("expected invalidParameter APIError, got %T: %v", err, err)
	}
	if called {
		t.Error("callback fired for an envelope without a refill rate")
	}
	if s := c.Tokens(); s.Left != 1199 || s.RefillRate != 20 {
		t.Errorf("bucket overwritten with zeros: %+v", s)
	}
}

func TestDoIgnoresNonEnvelopeBody(t *testing.T) {
	called := false
	c, _ := newTestClient(t, serveJSON(http.StatusBadGateway, `{"message":"gateway blew up"}`), WithTokenCallback(func(u TokenUpdate) {
		if u.Path != "/token" {
			called = true
		}
	}))
	c.tokens.state = TokenState{Known: true, Left: 1200, RefillRate: 20, UpdatedAt: testNow}
	_, err := probe(t.Context(), c, request{cost: 5})
	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok || httpErr.StatusCode != 502 || !bytes.Contains(httpErr.Body, []byte("gateway")) {
		t.Fatalf("expected *HTTPError 502, got %T: %v", err, err)
	}
	if called {
		t.Error("callback fired for a body that is not a Keepa envelope")
	}
	// The reservation took 5; nothing else may have changed.
	if s := c.Tokens(); s.Left != 1195 || s.RefillRate != 20 {
		t.Errorf("bucket = %+v, want Left 1195", s)
	}
}

func TestDoNon200NonJSON(t *testing.T) {
	c, _ := newTestClient(t, serveJSON(500, "Internal Server Error"))
	_, err := probe(t.Context(), c, request{cost: 1})
	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok || httpErr.StatusCode != 500 {
		t.Fatalf("expected *HTTPError 500, got %T: %v", err, err)
	}
}

func TestDoUndecodable200(t *testing.T) {
	logger, logs := captureLogs()
	c, _ := newTestClient(t, serveJSON(200, `<html>oops</html>`), WithLogger(logger))
	c.tokens.state = TokenState{Known: true, Left: 1200, RefillRate: 20, UpdatedAt: testNow}
	_, err := probe(t.Context(), c, request{cost: 1})
	if err == nil || !strings.Contains(err.Error(), "keepa: decoding /probe response") {
		t.Fatalf("err = %v", err)
	}
	if s := c.Tokens(); s.Left != 1199 {
		t.Errorf("bucket changed beyond the reservation: %+v", s)
	}
	if !strings.Contains(logs.String(), "keepa: undecodable response") {
		t.Errorf("not logged: %s", logs.String())
	}
}

func TestDoWarnsOnCostMismatch(t *testing.T) {
	logger, logs := captureLogs()
	c, _ := newTestClient(t, serveJSON(200, okEnvelope("")), WithLogger(logger)) // reports 1 consumed
	if _, err := probe(t.Context(), c, request{cost: 0}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "keepa: token cost mismatch") {
		t.Errorf("expected a mismatch warning: %s", logs.String())
	}
	logs.Reset()
	if _, err := probe(t.Context(), c, request{cost: 1}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), "mismatch") {
		t.Errorf("no warning expected when actual <= expected: %s", logs.String())
	}
}

func TestDoTokenCallbackFields(t *testing.T) {
	var got TokenUpdate
	c, _ := newTestClient(t, serveJSON(200, okEnvelope("")), WithTokenCallback(func(u TokenUpdate) {
		if u.Path == "/probe" {
			got = u
		}
	}))
	if _, err := probe(t.Context(), c, request{cost: 1}); err != nil {
		t.Fatal(err)
	}
	if got.Left != 1195 || got.Consumed != 1 || got.RefillRate != 20 || got.RefillIn != 30*time.Second || got.FlowReduction != 0 {
		t.Errorf("TokenUpdate = %+v", got)
	}
	if !got.Timestamp.Equal(time.UnixMilli(1790277678673)) {
		t.Errorf("Timestamp = %v", got.Timestamp)
	}
}

func TestDoFallbackTimeoutOnlyWithoutDeadline(t *testing.T) {
	slow := func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(150 * time.Millisecond):
		}
		serveJSON(200, okEnvelope(""))(w, r)
	}
	c, _ := newTestClient(t, slow, WithTimeout(20*time.Millisecond))
	if _, err := probe(context.Background(), c, request{cost: 1}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("without a caller deadline the client timeout applies; got %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := probe(ctx, c, request{cost: 1}); err != nil {
		t.Errorf("a caller deadline must win over the client timeout: %v", err)
	}
	if _, err := probe(context.Background(), c, request{cost: 1, timeout: 2 * time.Second}); err != nil {
		t.Errorf("a per-request timeout must override the client default: %v", err)
	}
}

func TestDoUsesLimiter(t *testing.T) {
	lim := &fakeLimiter{}
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")), WithLimiter(lim))
	if _, err := probe(t.Context(), c, request{cost: 1}); err != nil {
		t.Fatal(err)
	}
	if lim.calls == 0 {
		t.Error("limiter not consulted")
	}
	before := len(rec.Calls())
	lim.err = errors.New("limited")
	if _, err := probe(t.Context(), c, request{cost: 1}); !errors.Is(err, lim.err) {
		t.Errorf("limiter error not returned: %v", err)
	}
	if len(rec.Calls()) != before {
		t.Error("request sent despite the limiter refusing")
	}
}

func TestDoHonoursPerCallReserve(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")), WithTokenReserve(1000))
	c.tokens.state = TokenState{Known: true, Left: 500, RefillRate: 20, UpdatedAt: testNow}
	_, err := probe(t.Context(), c, request{cost: 5, callParams: callParams{noWait: true}})
	if !errors.Is(err, ErrWouldWait) {
		t.Fatalf("client floor of 1000 should hold the call: %v", err)
	}
	if len(rec.Calls()) != 0 {
		t.Error("request reached the server despite the reserve")
	}
	zero := 0
	if _, err := probe(t.Context(), c, request{cost: 5, callParams: callParams{noWait: true, reserve: &zero}}); err != nil {
		t.Errorf("WithReserve(0) should let the call through: %v", err)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./... 2>&1 | head -20`
Expected: build failure mentioning `undefined: request` or `undefined: do`.

- [ ] **Step 4: Write the implementation**

`transport.go`:

```go
package keepa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Envelope is the metadata Keepa includes in every response, including
// error responses. Every response type embeds it.
type Envelope struct {
	Timestamp          int64     `json:"timestamp"`          // server time, Unix milliseconds
	TokensLeft         int       `json:"tokensLeft"`         // may be negative
	TokensConsumed     int       `json:"tokensConsumed"`
	RefillIn           int       `json:"refillIn"`           // milliseconds until the next refill
	RefillRate         int       `json:"refillRate"`         // tokens per minute
	TokenFlowReduction float64   `json:"tokenFlowReduction"` // tokens per minute lost to tracking
	ProcessingTimeInMs int       `json:"processingTimeInMs"`
	Error              *APIError `json:"error"` // StatusCode is set by the client
}

// isEnvelope distinguishes a Keepa body from an unrelated JSON error page.
func (e *Envelope) isEnvelope() bool { return e.Timestamp > 0 || e.RefillRate > 0 }

// request describes one API call.
type request struct {
	path    string        // "/product"
	query   url.Values    // includes the API key
	body    any           // JSON-encoded and POSTed when non-nil
	cost    int           // expected token cost
	timeout time.Duration // fallback deadline; 0 means the client default
	callParams
}

// query returns query values with the API key set.
func (c *Client) query() url.Values {
	vals := url.Values{}
	vals.Set("key", c.apiKey)
	return vals
}

// do executes r and decodes the body into T.
func do[T any](ctx context.Context, c *Client, r request) (*T, error) {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}
	}

	reserve := c.reserve
	if r.reserve != nil {
		reserve = *r.reserve
	}
	if err := c.waitForTokens(ctx, r.cost, reserve, r.noWait); err != nil {
		return nil, err
	}

	if _, ok := ctx.Deadline(); !ok {
		timeout := r.timeout
		if timeout == 0 {
			timeout = c.timeout
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	req, err := c.newRequest(ctx, r)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// The envelope is read before the status check: an error response,
	// notably the 429 when the quota runs out, still carries the
	// authoritative token counts. An envelope without a refill rate (Keepa
	// sends zeros with parameter errors) carries no reading and is skipped.
	var env Envelope
	decodeErr := json.Unmarshal(body, &env)
	switch {
	case decodeErr == nil && env.isEnvelope():
		if env.RefillRate > 0 {
			c.recordEnvelope(&env, r.path)
			if env.TokensConsumed > r.cost {
				c.logger.Warn("keepa: token cost mismatch", "path", r.path, "expected", r.cost, "actual", env.TokensConsumed)
			}
		}
		if env.Error != nil {
			env.Error.StatusCode = resp.StatusCode
			return nil, env.Error
		}
	case decodeErr != nil:
		c.logger.Warn("keepa: undecodable response", "path", r.path, "status", resp.StatusCode, "body", truncate(body, 512))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: body}
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("keepa: decoding %s response: %w", r.path, decodeErr)
	}

	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("keepa: decoding %s response: %w", r.path, err)
	}
	return &out, nil
}

func (c *Client) newRequest(ctx context.Context, r request) (*http.Request, error) {
	u := c.baseURL + r.path + "?" + r.query.Encode()
	if r.body == nil {
		return http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	}
	b, err := json.Marshal(r.body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}
```

Append to `tokens.go`:

```go
// recordEnvelope syncs the bucket with an envelope and notifies the callback.
func (c *Client) recordEnvelope(env *Envelope, path string) {
	now := c.now()
	c.tokens.mu.Lock()
	s := &c.tokens.state
	s.Known = true
	s.Left = env.TokensLeft
	s.RefillRate = env.RefillRate
	s.FlowReduction = env.TokenFlowReduction
	if now.After(s.UpdatedAt) {
		s.UpdatedAt = now
	}
	c.tokens.mu.Unlock()

	if c.onTokens != nil {
		c.onTokens(TokenUpdate{
			Left:          env.TokensLeft,
			Consumed:      env.TokensConsumed,
			RefillRate:    env.RefillRate,
			RefillIn:      time.Duration(env.RefillIn) * time.Millisecond,
			FlowReduction: env.TokenFlowReduction,
			Path:          path,
			Timestamp:     time.UnixMilli(env.Timestamp),
		})
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test -race -count=1 ./...`
Expected: `ok`. `serveFixture` is unused until Task 8; that is fine for a test file.

- [ ] **Step 6: Commit**

```bash
git -C /Users/jameseagle/code/Jleagle/keepa add transport.go tokens.go testhelpers_test.go transport_test.go
git -C /Users/jameseagle/code/Jleagle/keepa commit -m "Add request transport with envelope-first handling"
```

---

### Task 7: Token status endpoint and seeding

**Files:**
- Modify: `tokens.go` (add `seed`, `GetTokenStatus`, `TokenResponse`), `transport.go` (call `seed` for paid requests), `testhelpers_test.go` (add `costOf`)
- Test: `tokens_test.go` (append seeding tests)

**Interfaces:**
- Produces: `func (c *Client) GetTokenStatus(ctx context.Context) (*TokenResponse, error)`; `type TokenResponse struct{ Envelope }`; unexported `func (c *Client) seed(ctx context.Context)`; test helper `costOf(t, err) int`.

- [ ] **Step 1: Add the test helper**

Append to `testhelpers_test.go` (add `"errors"` to its imports):

```go
// costOf returns the token cost the client computed for a call made with
// WithReserve(1200) and WithoutWaiting(): the seeded bucket holds exactly
// 1200, so any positive cost forces a *TokenWaitError that carries it, and
// no request reaches the endpoint.
func costOf(t *testing.T, err error) int {
	t.Helper()
	werr, ok := errors.AsType[*TokenWaitError](err)
	if !ok {
		t.Fatalf("expected *TokenWaitError, got %T: %v", err, err)
	}
	return werr.Cost
}
```

- [ ] **Step 2: Write the failing tests**

Append to `tokens_test.go` (add `"slices"` and `"sync"` to its imports):

```go
func TestSeedRunsBeforeFirstPaidCallOnly(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	for range 2 {
		if _, err := probe(t.Context(), c, request{cost: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if got := rec.Paths(); !slices.Equal(got, []string{"/token", "/probe", "/probe"}) {
		t.Errorf("paths = %v, want one seed then two probes", got)
	}
}

func TestSeedAppliesReserveToFirstCall(t *testing.T) {
	// The fixture reports 1200 tokens. With a floor of 1200 the very first
	// paid call is already held back, and no probe reaches the server.
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")), WithTokenReserve(1200))
	_, err := probe(t.Context(), c, request{cost: 5, callParams: callParams{noWait: true}})
	if !errors.Is(err, ErrWouldWait) {
		t.Fatalf("err = %v, want ErrWouldWait", err)
	}
	if got := costOf(t, err); got != 5 {
		t.Errorf("cost = %d, want 5", got)
	}
	if got := rec.Paths(); !slices.Equal(got, []string{"/token"}) {
		t.Errorf("paths = %v, want only the seed", got)
	}
}

func TestSeedFailureIsLoggedAndIgnored(t *testing.T) {
	logger, logs := captureLogs()
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")), WithLogger(logger))
	rec.SetTokenHandler(serveJSON(500, "boom"))
	if _, err := probe(t.Context(), c, request{cost: 1}); err != nil {
		t.Fatalf("the paid call must proceed when the seed fails: %v", err)
	}
	if !strings.Contains(logs.String(), "keepa: token seed failed") {
		t.Errorf("seed failure not logged: %s", logs.String())
	}
	// The probe's envelope synced the bucket, so no further seed is needed.
	if _, err := probe(t.Context(), c, request{cost: 1}); err != nil {
		t.Fatal(err)
	}
	if got := rec.Paths(); !slices.Equal(got, []string{"/token", "/probe", "/probe"}) {
		t.Errorf("paths = %v", got)
	}
}

func TestSeedRetriesWhileStateUnknown(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(502, `{"message":"down"}`))
	rec.SetTokenHandler(serveJSON(500, "boom"))
	for range 2 {
		_, _ = probe(t.Context(), c, request{cost: 1})
	}
	if got := rec.Paths(); !slices.Equal(got, []string{"/token", "/probe", "/token", "/probe"}) {
		t.Errorf("paths = %v, want a seed attempt before each call while unknown", got)
	}
}

func TestSeedDoesNotRecurse(t *testing.T) {
	// GetTokenStatus costs nothing, so it never triggers a seed of its own.
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	res, err := c.GetTokenStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.TokensLeft != 1200 || res.RefillRate != 20 {
		t.Errorf("TokenResponse = %+v", res.Envelope)
	}
	if got := rec.Paths(); !slices.Equal(got, []string{"/token"}) {
		t.Errorf("paths = %v, want exactly one /token", got)
	}
	if s := c.Tokens(); !s.Known || s.Left != 1200 || s.RefillRate != 20 {
		t.Errorf("bucket = %+v", s)
	}
	if got := rec.Calls()[0].query.Get("key"); got != "test-key" {
		t.Errorf("key = %q", got)
	}
}

func TestSeedBypassesReserve(t *testing.T) {
	// The free status call goes through even when the bucket is below the floor.
	c, _ := newTestClient(t, serveJSON(200, okEnvelope("")), WithTokenReserve(5000))
	if _, err := c.GetTokenStatus(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := probe(t.Context(), c, request{cost: 1, callParams: callParams{noWait: true}}); !errors.Is(err, ErrWouldWait) {
		t.Errorf("a paid call should be held by the floor: %v", err)
	}
}

func TestSeedRunsOnceUnderConcurrency(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { _, _ = probe(t.Context(), c, request{cost: 1}) })
	}
	wg.Wait()
	seeds := 0
	for _, p := range rec.Paths() {
		if p == "/token" {
			seeds++
		}
	}
	if seeds != 1 {
		t.Errorf("seeded %d times, want 1", seeds)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./... 2>&1 | head -20`
Expected: build failure mentioning `GetTokenStatus`.

- [ ] **Step 4: Write the implementation**

Append to `tokens.go`:

```go
// seed fetches the token status once so the reserve applies to the first
// paid call. Concurrent callers do not wait for a seed already in flight; a
// failed seed is logged and retried on the next paid call.
func (c *Client) seed(ctx context.Context) {
	c.tokens.mu.Lock()
	if c.tokens.state.Known || c.tokens.seeding {
		c.tokens.mu.Unlock()
		return
	}
	c.tokens.seeding = true
	c.tokens.mu.Unlock()

	_, err := c.GetTokenStatus(ctx)

	c.tokens.mu.Lock()
	c.tokens.seeding = false
	c.tokens.mu.Unlock()

	if err != nil {
		c.logger.Warn("keepa: token seed failed", "error", err)
	}
}

// TokenResponse is returned by GetTokenStatus. Only the Envelope is populated.
type TokenResponse struct {
	Envelope
}

// GetTokenStatus retrieves the token bucket state. Cost: 0 tokens, so it is
// never held back by the reserve.
func (c *Client) GetTokenStatus(ctx context.Context) (*TokenResponse, error) {
	return do[TokenResponse](ctx, c, request{path: "/token", query: c.query(), cost: 0})
}
```

In `transport.go`, make the first statement of `do` seed the bucket for paid requests:

```go
func do[T any](ctx context.Context, c *Client, r request) (*T, error) {
	if r.cost > 0 {
		c.seed(ctx)
	}

	if c.limiter != nil {
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test -race -count=1 ./...`
Expected: `ok`. Every transport test from Task 6 still passes because they either filter `/token` out of callbacks and recorded calls or pre-set `Known`.

- [ ] **Step 6: Commit**

```bash
git -C /Users/jameseagle/code/Jleagle/keepa add tokens.go transport.go testhelpers_test.go tokens_test.go
git -C /Users/jameseagle/code/Jleagle/keepa commit -m "Add token status endpoint and bucket seeding"
```

---

### Task 8: Products endpoint

**Files:**
- Create: `product.go`, `product_types.go`, `testdata/product.json`
- Test: `product_test.go`

**Interfaces:**
- Consumes: `do`, `request`, `productParams`, `productOption`, `ProductOption`, `Domain`, `Time`, `History`, `CSV`, `invalidRequest`, test helpers.
- Produces: `func (c *Client) GetProducts(ctx context.Context, domain Domain, asins []string, opts ...ProductOption) (*ProductResponse, error)`; options `WithStats(time.Time)`, `WithRatings()`, `WithLiveUpdate()`, `WithBuyBox()`, `WithVideos()`, `WithoutHistory()`; types `ProductResponse`, `Product` (with `LastCategory()`), `ProductStats`, `ProductReviews`, `CategoryNode`, `Image`, `Variation`, `VariationAttribute` (also used by `LightningDeal` in Task 12), `Video`, `FBAFees`, `HazardousMaterial`, `UnitCount`; unexported `keepaStart`.

- [ ] **Step 1: Write the fixture**

`testdata/product.json`:

```json
{
  "timestamp": 1790277678673,
  "tokensLeft": 1195,
  "tokensConsumed": 1,
  "refillIn": 30000,
  "refillRate": 20,
  "tokenFlowReduction": 0,
  "processingTimeInMs": 12,
  "products": [
    {
      "asin": "B07XJ8C8F5",
      "domainId": 1,
      "title": "Test Product",
      "brand": "Acme",
      "trackingSince": 4000000,
      "listedSince": 3900000,
      "lastUpdate": 7204320,
      "lastPriceChange": 7204000,
      "lastRatingUpdate": 7204000,
      "releaseDate": 20190801,
      "publicationDate": -1,
      "categories": [172282, 502394],
      "rootCategory": 172282,
      "categoryTree": [
        {"catId": 172282, "name": "Electronics"},
        {"catId": 502394, "name": "Camera & Photo"}
      ],
      "images": [
        {"l": "51abc.jpg", "lH": 1000, "lW": 1000, "m": "51abc._SL500_.jpg", "mH": 500, "mW": 500}
      ],
      "variations": [
        {"asin": "B07XJ8C8F6", "image": "51def.jpg", "attributes": [{"dimension": "Color", "value": "Black"}]}
      ],
      "videos": [
        {"title": "Demo", "image": "vid.jpg", "duration": 30, "creator": "Acme", "name": "demo", "url": "https://example.com/v.mp4"}
      ],
      "csv": [[7200000, 1999, 7204320, 1899], [7200000, 1899], null, [7200000, 1234]],
      "salesRanks": {"172282": [7200000, 1234]},
      "stats": {
        "current": [1899, 1899, -1, 1234],
        "avg30": [1950, 1900, -1, 1300],
        "min": [[7204320, 1899], null],
        "max": [[7200000, 1999], null],
        "lastOffersUpdate": 7204320,
        "buyBoxPrice": 1899,
        "buyBoxIsAmazon": true,
        "buyBoxSellerId": "ATVPDKIKX0DER",
        "totalOfferCount": 5
      },
      "reviews": {"lastUpdate": 7204000, "reviewCount": [7200000, 120], "ratingCount": [7200000, 400]},
      "monthlySold": 500,
      "hasReviews": true
    }
  ]
}
```

- [ ] **Step 2: Write the failing tests**

`product_test.go`:

```go
package keepa

import (
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"
)

func TestGetProductsRequestDefaults(t *testing.T) {
	c, rec := newTestClient(t, serveFixture(t, "product.json"))
	res, err := c.GetProducts(t.Context(), DomainGB, []string{"B07XJ8C8F5", "B07XJ8C8F6"})
	if err != nil {
		t.Fatal(err)
	}
	last := rec.Last(t)
	if last.method != http.MethodGet || last.path != "/product" {
		t.Errorf("request = %s %s", last.method, last.path)
	}
	want := url.Values{"key": {"test-key"}, "domain": {"2"}, "asin": {"B07XJ8C8F5,B07XJ8C8F6"}}
	if !maps.EqualFunc(last.query, want, slices.Equal) {
		t.Errorf("query = %v, want exactly %v", last.query, want)
	}
	if len(res.Products) != 1 || res.TokensLeft != 1195 {
		t.Errorf("response = %d products, %d tokens left", len(res.Products), res.TokensLeft)
	}
}

func TestGetProductsOptions(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		opts []ProductOption
		want map[string]string
	}{
		{"stats", []ProductOption{WithStats(since)}, map[string]string{"stats": "1767225600000,1790251200000"}},
		{"stats clamped to 2011", []ProductOption{WithStats(time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC))}, map[string]string{"stats": "1293840000000,1790251200000"}},
		{"stats zero time clamped", []ProductOption{WithStats(time.Time{})}, map[string]string{"stats": "1293840000000,1790251200000"}},
		{"ratings", []ProductOption{WithRatings()}, map[string]string{"rating": "1"}},
		{"live update", []ProductOption{WithLiveUpdate()}, map[string]string{"update": "0"}},
		{"buy box", []ProductOption{WithBuyBox()}, map[string]string{"buybox": "1"}},
		{"videos", []ProductOption{WithVideos()}, map[string]string{"videos": "1"}},
		{"no history", []ProductOption{WithoutHistory()}, map[string]string{"history": "0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := newTestClient(t, serveJSON(200, okEnvelope(`"products":[]`)))
			if _, err := c.GetProducts(t.Context(), DomainUS, []string{"B07XJ8C8F5"}, tt.opts...); err != nil {
				t.Fatal(err)
			}
			q := rec.Last(t).query
			for k, v := range tt.want {
				if got := q.Get(k); got != v {
					t.Errorf("%s = %q, want %q", k, got, v)
				}
			}
			if len(q) != 3+len(tt.want) {
				t.Errorf("unexpected parameters sent: %v", q)
			}
		})
	}
}

func TestGetProductsCost(t *testing.T) {
	asins := []string{"A", "B", "C"}
	tests := []struct {
		name string
		opts []ProductOption
		want int
	}{
		{"base", nil, 3},
		{"ratings", []ProductOption{WithRatings()}, 6},
		{"live update", []ProductOption{WithLiveUpdate()}, 6},
		{"buy box", []ProductOption{WithBuyBox()}, 9},
		{"free options", []ProductOption{WithStats(testNow), WithVideos(), WithoutHistory()}, 3},
		{"everything", []ProductOption{WithRatings(), WithLiveUpdate(), WithBuyBox()}, 15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestClient(t, serveJSON(200, okEnvelope("")))
			opts := append(slices.Clone(tt.opts), WithReserve(1200), WithoutWaiting())
			_, err := c.GetProducts(t.Context(), DomainUS, asins, opts...)
			if got := costOf(t, err); got != tt.want {
				t.Errorf("cost = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGetProductsValidation(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	tests := []struct {
		name   string
		domain Domain
		asins  []string
	}{
		{"no asins", DomainUS, nil},
		{"too many asins", DomainUS, make([]string, 101)},
		{"reserved domain", Domain(7), []string{"A"}},
		{"zero domain", 0, []string{"A"}},
	}
	for _, tt := range tests {
		if _, err := c.GetProducts(t.Context(), tt.domain, tt.asins); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: err = %v, want ErrInvalidRequest", tt.name, err)
		}
	}
	if n := len(rec.Calls()); n != 0 {
		t.Errorf("%d requests sent; validation must run before the seed and the call", n)
	}
	if _, err := c.GetProducts(t.Context(), DomainUS, make([]string, 100)); err != nil {
		t.Errorf("100 ASINs must be allowed: %v", err)
	}
}

func TestGetProductsDecodes(t *testing.T) {
	c, _ := newTestClient(t, serveFixture(t, "product.json"))
	res, err := c.GetProducts(t.Context(), DomainUS, []string{"B07XJ8C8F5"})
	if err != nil {
		t.Fatal(err)
	}
	p := res.Products[0]
	if p.ASIN != "B07XJ8C8F5" || p.Title != "Test Product" || p.Brand != "Acme" || p.DomainID != 1 {
		t.Errorf("basic fields: %+v", p)
	}
	if p.LastUpdate != 7204320 || !p.LastUpdate.Valid() || p.TrackingSince != 4000000 || p.ListedSince != 3900000 {
		t.Errorf("time fields: lastUpdate=%d trackingSince=%d listedSince=%d", p.LastUpdate, p.TrackingSince, p.ListedSince)
	}
	if p.ReleaseDate != 20190801 || p.PublicationDate != -1 {
		t.Errorf("date fields: release=%d publication=%d", p.ReleaseDate, p.PublicationDate)
	}
	if !slices.Equal(p.CSV.Amazon, History{7200000, 1999, 7204320, 1899}) || p.CSV.Used != nil || !slices.Equal(p.CSV.Sales, History{7200000, 1234}) {
		t.Errorf("csv: amazon=%v used=%v sales=%v", p.CSV.Amazon, p.CSV.Used, p.CSV.Sales)
	}
	if !slices.Equal(p.SalesRanks[172282], []int64{7200000, 1234}) {
		t.Errorf("salesRanks = %v", p.SalesRanks)
	}
	if p.Stats == nil {
		t.Fatal("stats missing")
	}
	if p.Stats.Current[CSVAmazon] != 1899 || p.Stats.Current[CSVSales] != 1234 || p.Stats.LastOffersUpdate != 7204320 {
		t.Errorf("stats: %+v", p.Stats)
	}
	if !slices.Equal(p.Stats.Min[CSVAmazon], []int{7204320, 1899}) || p.Stats.Min[CSVNew] != nil {
		t.Errorf("stats min = %v", p.Stats.Min)
	}
	if p.Stats.BuyBoxIsAmazon == nil || !*p.Stats.BuyBoxIsAmazon || p.Stats.BuyBoxSellerID == nil || *p.Stats.BuyBoxSellerID != "ATVPDKIKX0DER" {
		t.Errorf("buy box: %+v", p.Stats)
	}
	if got := p.LastCategory(); got != (CategoryNode{CatID: 502394, Name: "Camera & Photo"}) {
		t.Errorf("LastCategory() = %+v", got)
	}
	if len(p.Variations) != 1 || p.Variations[0].ASIN != "B07XJ8C8F6" || p.Variations[0].Attributes[0] != (VariationAttribute{Dimension: "Color", Value: "Black"}) {
		t.Errorf("variations = %+v", p.Variations)
	}
	if len(p.Videos) != 1 || p.Videos[0].URL != "https://example.com/v.mp4" || p.Videos[0].Duration != 30 {
		t.Errorf("videos = %+v", p.Videos)
	}
	if p.Reviews.LastUpdate != 7204000 || !slices.Equal(p.Reviews.ReviewCount, History{7200000, 120}) {
		t.Errorf("reviews = %+v", p.Reviews)
	}
	if len(p.Images) != 1 || p.Images[0].L != "51abc.jpg" || p.Images[0].MH != 500 {
		t.Errorf("images = %+v", p.Images)
	}
	if p.MonthlySold != 500 || !p.HasReviews {
		t.Errorf("monthlySold=%d hasReviews=%v", p.MonthlySold, p.HasReviews)
	}
}

func TestProductLastCategoryEmpty(t *testing.T) {
	if got := (Product{}).LastCategory(); got != (CategoryNode{}) {
		t.Errorf("LastCategory() on empty tree = %+v", got)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./... 2>&1 | head -20`
Expected: build failure mentioning `GetProducts`.

- [ ] **Step 4: Write the implementation**

`product.go`:

```go
package keepa

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// keepaStart is the earliest date Keepa holds data for. WithStats clamps to it.
var keepaStart = time.Date(2011, 1, 1, 0, 0, 0, 0, time.UTC)

// WithStats requests the statistics object computed over the interval from
// since until now. Dates before 2011 are clamped to 2011-01-01. No extra tokens.
func WithStats(since time.Time) ProductOption {
	return productOption(func(p *productParams) { p.stats = &since })
}

// WithRatings includes rating and review count history. One extra token per ASIN.
func WithRatings() ProductOption {
	return productOption(func(p *productParams) { p.ratings = true })
}

// WithLiveUpdate asks Keepa to refresh each product before answering
// (update=0). One extra token per ASIN, which Keepa waives when its data is
// already fresh.
func WithLiveUpdate() ProductOption {
	return productOption(func(p *productParams) { p.liveUpdate = true })
}

// WithBuyBox adds Buy Box data to the statistics object. Two extra tokens per ASIN.
func WithBuyBox() ProductOption {
	return productOption(func(p *productParams) { p.buyBox = true })
}

// WithVideos includes video metadata. No extra tokens.
func WithVideos() ProductOption {
	return productOption(func(p *productParams) { p.videos = true })
}

// WithoutHistory omits the csv history arrays from the response. No extra tokens.
func WithoutHistory() ProductOption {
	return productOption(func(p *productParams) { p.noHistory = true })
}

// GetProducts retrieves between 1 and 100 products by ASIN.
// Cost: 1 token per ASIN plus the extras documented on each option.
func (c *Client) GetProducts(ctx context.Context, domain Domain, asins []string, opts ...ProductOption) (*ProductResponse, error) {
	if !domain.Valid() {
		return nil, invalidRequest("unknown domain %d", domain)
	}
	if len(asins) == 0 || len(asins) > 100 {
		return nil, invalidRequest("between 1 and 100 ASINs required, got %d", len(asins))
	}
	var p productParams
	for _, o := range opts {
		o.applyProduct(&p)
	}

	q := c.query()
	q.Set("domain", domain.queryValue())
	q.Set("asin", strings.Join(asins, ","))
	perASIN := 1
	if p.stats != nil {
		since := *p.stats
		if since.Before(keepaStart) {
			since = keepaStart
		}
		q.Set("stats", strconv.FormatInt(since.UnixMilli(), 10)+","+strconv.FormatInt(c.now().UnixMilli(), 10))
	}
	if p.ratings {
		q.Set("rating", "1")
		perASIN++
	}
	if p.liveUpdate {
		q.Set("update", "0")
		perASIN++
	}
	if p.buyBox {
		q.Set("buybox", "1")
		perASIN += 2
	}
	if p.videos {
		q.Set("videos", "1")
	}
	if p.noHistory {
		q.Set("history", "0")
	}

	return do[ProductResponse](ctx, c, request{
		path:       "/product",
		query:      q,
		cost:       len(asins) * perASIN,
		callParams: p.callParams,
	})
}

// ProductResponse is returned by GetProducts.
type ProductResponse struct {
	Envelope
	Products []Product `json:"products"`
}
```

`product_types.go` (run `gofmt -w` after pasting; field alignment is not shown exactly):

```go
package keepa

// Product is a Keepa product object. Prices are integers in the smallest
// unit of the marketplace currency; -1 means no data. Timestamps Keepa
// documents as Keepa minutes are typed Time. PublicationDate and ReleaseDate
// are YYYYMMDD integers, -1 when unknown.
type Product struct {
	ASIN                            string              `json:"asin"`
	Author                          *string             `json:"author"`
	AvailabilityAmazon              int                 `json:"availabilityAmazon"`
	AvailabilityAmazonDelay         []int               `json:"availabilityAmazonDelay"`
	BatteriesIncluded               bool                `json:"batteriesIncluded"`
	BatteriesRequired               bool                `json:"batteriesRequired"`
	Binding                         string              `json:"binding"`
	Brand                           string              `json:"brand"`
	BrandStoreName                  string              `json:"brandStoreName"`
	BrandStoreURL                   string              `json:"brandStoreUrl"`
	BrandStoreURLName               string              `json:"brandStoreUrlName"`
	BuyBoxEligibleOfferCounts       []int               `json:"buyBoxEligibleOfferCounts"`
	BuyBoxSellerIDHistory           any                 `json:"buyBoxSellerIdHistory"`
	Categories                      []int64             `json:"categories"`
	CategoryTree                    []CategoryNode      `json:"categoryTree"`
	Color                           string              `json:"color"`
	CompetitivePriceThreshold       int                 `json:"competitivePriceThreshold"`
	Contributors                    [][]string          `json:"contributors,omitempty"`
	Coupon                          any                 `json:"coupon"`
	CSV                             CSV                 `json:"csv"`
	Description                     string              `json:"description"`
	DomainID                        int                 `json:"domainId"`
	EANList                         []string            `json:"eanList"`
	EbayListingIDs                  []int64             `json:"ebayListingIds"`
	Edition                         string              `json:"edition"`
	FBAFees                         *FBAFees            `json:"fbaFees"`
	Features                        []string            `json:"features"`
	Format                          any                 `json:"format"`
	FrequentlyBoughtTogether        any                 `json:"frequentlyBoughtTogether"`
	G                               int                 `json:"g"`
	GTINList                        []string            `json:"gtinList"`
	HasReviews                      bool                `json:"hasReviews"`
	HazardousMaterials              []HazardousMaterial `json:"hazardousMaterials"`
	Images                          []Image             `json:"images"`
	IncludedComponents              string              `json:"includedComponents"`
	IsAdultProduct                  bool                `json:"isAdultProduct"`
	IsEligibleForSuperSaverShipping bool                `json:"isEligibleForSuperSaverShipping"`
	IsEligibleForTradeIn            bool                `json:"isEligibleForTradeIn"`
	IsHeatSensitive                 bool                `json:"isHeatSensitive"`
	IsRedirectASIN                  bool                `json:"isRedirectASIN"`
	IsSNS                           bool                `json:"isSNS"`
	ItemHeight                      int                 `json:"itemHeight"`
	ItemLength                      int                 `json:"itemLength"`
	ItemWeight                      int                 `json:"itemWeight"`
	ItemWidth                       int                 `json:"itemWidth"`
	Languages                       [][]string          `json:"languages"`
	LastEbayUpdate                  Time                `json:"lastEbayUpdate"`
	LastPriceChange                 Time                `json:"lastPriceChange"`
	LastRatingUpdate                Time                `json:"lastRatingUpdate"`
	LastSoldUpdate                  Time                `json:"lastSoldUpdate"`
	LastUpdate                      Time                `json:"lastUpdate"`
	Launchpad                       bool                `json:"launchpad"`
	ListedSince                     Time                `json:"listedSince"`
	LiveOffersOrder                 any                 `json:"liveOffersOrder"`
	Manufacturer                    string              `json:"manufacturer"`
	Model                           string              `json:"model"`
	MonthlySold                     int                 `json:"monthlySold"`
	MonthlySoldHistory              []int               `json:"monthlySoldHistory"`
	NumberOfItems                   int                 `json:"numberOfItems"`
	NumberOfPages                   int                 `json:"numberOfPages"`
	OffersSuccessful                bool                `json:"offersSuccessful"`
	PackageHeight                   int                 `json:"packageHeight"`
	PackageLength                   int                 `json:"packageLength"`
	PackageQuantity                 int                 `json:"packageQuantity"`
	PackageWeight                   int                 `json:"packageWeight"`
	PackageWidth                    int                 `json:"packageWidth"`
	ParentASIN                      string              `json:"parentAsin"`
	ParentASINHistory               []string            `json:"parentAsinHistory"`
	ParentTitle                     string              `json:"parentTitle"`
	PartNumber                      string              `json:"partNumber"`
	PrimeDealEndTime                int                 `json:"primeDealEndTime"`
	ProductGroup                    string              `json:"productGroup"`
	ProductType                     int                 `json:"productType"`
	Promotions                      any                 `json:"promotions"`
	PublicationDate                 int                 `json:"publicationDate"`
	ReferralFeePercent              int                 `json:"referralFeePercent"`
	ReferralFeePercentage           float64             `json:"referralFeePercentage"`
	ReleaseDate                     int                 `json:"releaseDate"`
	Reviews                         ProductReviews      `json:"reviews"`
	RootCategory                    int64               `json:"rootCategory"`
	SalesRankDisplayGroup           string              `json:"salesRankDisplayGroup"`
	SalesRankReference              int                 `json:"salesRankReference"`
	SalesRankReferenceHistory       []int               `json:"salesRankReferenceHistory"`
	SalesRanks                      map[int64][]int64   `json:"salesRanks"`
	Size                            string              `json:"size"`
	SpecificUsesForProduct          []string            `json:"specificUsesForProduct"`
	Stats                           *ProductStats       `json:"stats"`
	Style                           string              `json:"style"`
	Title                           string              `json:"title"`
	TrackingSince                   Time                `json:"trackingSince"`
	Type                            string              `json:"type"`
	UnitCount                       UnitCount           `json:"unitCount"`
	UPCList                         []string            `json:"upcList"`
	URLSlug                         string              `json:"urlSlug"`
	VariableClosingFee              int                 `json:"variableClosingFee"`
	Variations                      []Variation         `json:"variations"`
	Videos                          []Video             `json:"videos"`
	WebsiteDisplayGroup             string              `json:"websiteDisplayGroup"`
	WebsiteDisplayGroupName         string              `json:"websiteDisplayGroupName"`
}

// LastCategory returns the deepest node of the category tree, or the zero
// value when the tree is empty.
func (p Product) LastCategory() CategoryNode {
	if len(p.CategoryTree) == 0 {
		return CategoryNode{}
	}
	return p.CategoryTree[len(p.CategoryTree)-1]
}

// CategoryNode is one level of a product's category tree.
type CategoryNode struct {
	CatID int64  `json:"catId"`
	Name  string `json:"name"`
}

// Image is one product image in large and medium sizes.
type Image struct {
	L  string `json:"l"`
	LH int    `json:"lH"`
	LW int    `json:"lW"`
	M  string `json:"m"`
	MH int    `json:"mH"`
	MW int    `json:"mW"`
}

// Variation is a sibling product that differs in the listed attributes.
type Variation struct {
	ASIN       string               `json:"asin"`
	Image      string               `json:"image"`
	Attributes []VariationAttribute `json:"attributes"`
}

// VariationAttribute is one dimension of a variation, such as Color: Black.
type VariationAttribute struct {
	Dimension string `json:"dimension"`
	Value     string `json:"value"`
}

// Video is a product video.
type Video struct {
	Title    string `json:"title"`
	Image    string `json:"image"`
	Duration int    `json:"duration"`
	Creator  string `json:"creator"`
	Name     string `json:"name"`
	URL      string `json:"url"`
}

// FBAFees holds Keepa's last known fulfilment fee.
type FBAFees struct {
	LastUpdate     int `json:"lastUpdate"`
	PickAndPackFee int `json:"pickAndPackFee"`
}

// HazardousMaterial is one hazardous material aspect of a product.
type HazardousMaterial struct {
	Aspect string `json:"aspect"`
	Value  string `json:"value"`
}

// ProductReviews holds review and rating count history.
type ProductReviews struct {
	LastUpdate  Time    `json:"lastUpdate"`
	ReviewCount History `json:"reviewCount"`
	RatingCount History `json:"ratingCount"`
}

// UnitCount is the unit a product is sold by.
type UnitCount struct {
	UnitType  string  `json:"unitType"`
	UnitValue float64 `json:"unitValue"`
}

// ProductStats is the statistics object returned when WithStats is used.
// Arrays are indexed by CSVType; min and max hold [time, value] pairs.
type ProductStats struct {
	AtIntervalStart                []int   `json:"atIntervalStart"`
	Avg                            []int   `json:"avg"`
	Avg180                         []int   `json:"avg180"`
	Avg30                          []int   `json:"avg30"`
	Avg365                         []int   `json:"avg365"`
	Avg90                          []int   `json:"avg90"`
	BuyBoxAvailabilityMessage      *string `json:"buyBoxAvailabilityMessage"`
	BuyBoxCondition                any     `json:"buyBoxCondition"`
	BuyBoxIsAmazon                 *bool   `json:"buyBoxIsAmazon"`
	BuyBoxIsBackorder              any     `json:"buyBoxIsBackorder"`
	BuyBoxIsFBA                    any     `json:"buyBoxIsFBA"`
	BuyBoxIsFreeShippingEligible   any     `json:"buyBoxIsFreeShippingEligible"`
	BuyBoxIsMAP                    any     `json:"buyBoxIsMAP"`
	BuyBoxIsPreorder               any     `json:"buyBoxIsPreorder"`
	BuyBoxIsPrimeEligible          *bool   `json:"buyBoxIsPrimeEligible"`
	BuyBoxIsPrimeExclusive         any     `json:"buyBoxIsPrimeExclusive"`
	BuyBoxIsShippable              any     `json:"buyBoxIsShippable"`
	BuyBoxIsUnqualified            any     `json:"buyBoxIsUnqualified"`
	BuyBoxIsUsed                   any     `json:"buyBoxIsUsed"`
	BuyBoxIsWarehouseDeal          any     `json:"buyBoxIsWarehouseDeal"`
	BuyBoxPrice                    int     `json:"buyBoxPrice"`
	BuyBoxSellerID                 *string `json:"buyBoxSellerId"`
	BuyBoxShipping                 int     `json:"buyBoxShipping"`
	BuyBoxShippingCountry          any     `json:"buyBoxShippingCountry"`
	Current                        []int   `json:"current"`
	DeltaPercent90MonthlySold      any     `json:"deltaPercent90_monthlySold"`
	IsLowest                       []bool  `json:"isLowest"`
	IsLowest90                     []bool  `json:"isLowest90"`
	LastBuyBoxUpdate               any     `json:"lastBuyBoxUpdate"`
	LastOffersUpdate               Time    `json:"lastOffersUpdate"`
	LightningDealInfo              any     `json:"lightningDealInfo"`
	Max                            [][]int `json:"max"`
	MaxInInterval                  [][]int `json:"maxInInterval"`
	Min                            [][]int `json:"min"`
	MinInInterval                  [][]int `json:"minInInterval"`
	OfferCountFBA                  int     `json:"offerCountFBA"`
	OfferCountFBM                  int     `json:"offerCountFBM"`
	OutOfStockCountAmazon30        int     `json:"outOfStockCountAmazon30"`
	OutOfStockCountAmazon90        int     `json:"outOfStockCountAmazon90"`
	OutOfStockPercentage180        []int   `json:"outOfStockPercentage180"`
	OutOfStockPercentage30         []int   `json:"outOfStockPercentage30"`
	OutOfStockPercentage365        []int   `json:"outOfStockPercentage365"`
	OutOfStockPercentage90         []int   `json:"outOfStockPercentage90"`
	OutOfStockPercentageInInterval []int   `json:"outOfStockPercentageInInterval"`
	RetrievedOfferCount            int     `json:"retrievedOfferCount"`
	SalesRankDrops180              int     `json:"salesRankDrops180"`
	SalesRankDrops30               int     `json:"salesRankDrops30"`
	SalesRankDrops365              int     `json:"salesRankDrops365"`
	SalesRankDrops90               int     `json:"salesRankDrops90"`
	SellerIDsLowestFBA             any     `json:"sellerIdsLowestFBA"`
	SellerIDsLowestFBM             any     `json:"sellerIdsLowestFBM"`
	TotalOfferCount                int     `json:"totalOfferCount"`
	TradeInPrice                   int     `json:"tradeInPrice"`
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `gofmt -w . && gofmt -l . && go vet ./... && go test -race -count=1 ./...`
Expected: `ok`

- [ ] **Step 6: Commit**

```bash
git -C /Users/jameseagle/code/Jleagle/keepa add product.go product_types.go product_test.go testdata/product.json
git -C /Users/jameseagle/code/Jleagle/keepa commit -m "Add products endpoint"
```

---

### Task 9: Best sellers endpoint

**Files:**
- Create: `bestsellers.go`, `testdata/bestsellers.json`
- Test: `bestsellers_test.go`

**Interfaces:**
- Consumes: `do`, `request`, `bestSellersParams`, `bestSellersOption` (defined in `options.go`), `BestSellersOption`, `Domain`, `Time`, `invalidRequest`, test helpers.
- Produces: `func (c *Client) GetBestSellers(ctx context.Context, domain Domain, categoryID int64, opts ...BestSellersOption) (*BestSellersResponse, error)`; options `WithRankRange(days int)`, `WithVariations()`; types `BestSellersResponse` (with `ASINs()`), `BestSellers`.

- [ ] **Step 1: Write the fixture**

`testdata/bestsellers.json` (the top-level list is what production receives; the nested list is what Keepa documents; they differ in length so tests can tell which one `ASINs()` chose):

```json
{
  "timestamp": 1790277678673,
  "tokensLeft": 1150,
  "tokensConsumed": 50,
  "refillIn": 30000,
  "refillRate": 20,
  "tokenFlowReduction": 0,
  "processingTimeInMs": 40,
  "bestSellersList": {
    "domainId": 1,
    "lastUpdate": 7204320,
    "categoryId": 172282,
    "asinList": ["B000000001", "B000000002"]
  },
  "asinList": ["B000000001", "B000000002", "B000000003"]
}
```

- [ ] **Step 2: Write the failing tests**

`bestsellers_test.go`:

```go
package keepa

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"
)

func TestGetBestSellersRequestDefaults(t *testing.T) {
	c, rec := newTestClient(t, serveFixture(t, "bestsellers.json"))
	res, err := c.GetBestSellers(t.Context(), DomainUS, 172282)
	if err != nil {
		t.Fatal(err)
	}
	last := rec.Last(t)
	if last.method != http.MethodGet || last.path != "/bestsellers" {
		t.Errorf("request = %s %s", last.method, last.path)
	}
	want := url.Values{"key": {"test-key"}, "domain": {"1"}, "category": {"172282"}}
	if !maps.EqualFunc(last.query, want, slices.Equal) {
		t.Errorf("query = %v, want exactly %v", last.query, want)
	}
	if res.TokensConsumed != 50 {
		t.Errorf("envelope not decoded: %+v", res.Envelope)
	}
}

func TestGetBestSellersOptions(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope(`"asinList":[]`)))
	if _, err := c.GetBestSellers(t.Context(), DomainUS, 172282, WithRankRange(90), WithVariations()); err != nil {
		t.Fatal(err)
	}
	q := rec.Last(t).query
	if q.Get("range") != "90" || q.Get("variations") != "1" || len(q) != 5 {
		t.Errorf("query = %v", q)
	}
}

func TestGetBestSellersValidation(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	if _, err := c.GetBestSellers(t.Context(), Domain(7), 1); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("reserved domain: err = %v", err)
	}
	if _, err := c.GetBestSellers(t.Context(), DomainUS, 1, WithRankRange(45)); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("range 45: err = %v", err)
	}
	if n := len(rec.Calls()); n != 0 {
		t.Errorf("%d requests sent before validation", n)
	}
	for _, days := range []int{30, 90, 180} {
		if _, err := c.GetBestSellers(t.Context(), DomainUS, 1, WithRankRange(days)); err != nil {
			t.Errorf("range %d should be allowed: %v", days, err)
		}
	}
}

func TestGetBestSellersCost(t *testing.T) {
	c, _ := newTestClient(t, serveJSON(200, okEnvelope("")))
	_, err := c.GetBestSellers(t.Context(), DomainUS, 1, WithReserve(1200), WithoutWaiting())
	if got := costOf(t, err); got != 50 {
		t.Errorf("cost = %d, want 50", got)
	}
}

func TestGetBestSellersDoublesTimeout(t *testing.T) {
	// With a 30ms client timeout, a 45ms response succeeds only because
	// best sellers doubles the fallback deadline.
	slow := func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(45 * time.Millisecond):
		}
		serveJSON(200, okEnvelope(`"asinList":[]`))(w, r)
	}
	c, _ := newTestClient(t, slow, WithTimeout(30*time.Millisecond))
	c.tokens.state = TokenState{Known: true, Left: 1200, RefillRate: 20, UpdatedAt: testNow}
	if _, err := c.GetBestSellers(t.Context(), DomainUS, 1); err != nil {
		t.Errorf("expected the doubled timeout to cover a 45ms response: %v", err)
	}
}

func TestGetBestSellersDecodesAndPrefersTopLevelList(t *testing.T) {
	c, _ := newTestClient(t, serveFixture(t, "bestsellers.json"))
	res, err := c.GetBestSellers(t.Context(), DomainUS, 172282)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.ASINs(); !slices.Equal(got, []string{"B000000001", "B000000002", "B000000003"}) {
		t.Errorf("ASINs() = %v, want the top-level list", got)
	}
	bl := res.BestSellersList
	if bl.DomainID != 1 || bl.CategoryID != 172282 || bl.LastUpdate != 7204320 || len(bl.ASINList) != 2 {
		t.Errorf("bestSellersList = %+v", bl)
	}
}

func TestBestSellersResponseFallsBackToNestedList(t *testing.T) {
	var res BestSellersResponse
	if err := json.Unmarshal([]byte(`{"bestSellersList":{"asinList":["X"]}}`), &res); err != nil {
		t.Fatal(err)
	}
	if got := res.ASINs(); !slices.Equal(got, []string{"X"}) {
		t.Errorf("ASINs() = %v, want the nested list", got)
	}
	if got := (&BestSellersResponse{}).ASINs(); len(got) != 0 {
		t.Errorf("ASINs() on empty = %v", got)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./... 2>&1 | head -20`
Expected: build failure mentioning `GetBestSellers`.

- [ ] **Step 4: Write the implementation**

`bestsellers.go`:

```go
package keepa

import (
	"context"
	"strconv"
)

// WithRankRange ranks by the average sales rank over the last 30, 90 or 180
// days instead of the current rank.
func WithRankRange(days int) BestSellersOption {
	return bestSellersOption(func(p *bestSellersParams) { p.rankRange = days })
}

// WithVariations lists every variation of a product instead of one per family.
func WithVariations() BestSellersOption {
	return bestSellersOption(func(p *bestSellersParams) { p.variations = true })
}

// GetBestSellers retrieves the best-selling ASINs of a category, best first.
// Cost: 50 tokens. Keepa charges nothing when it has no list for the category.
// The fallback timeout is doubled because the list can be large.
func (c *Client) GetBestSellers(ctx context.Context, domain Domain, categoryID int64, opts ...BestSellersOption) (*BestSellersResponse, error) {
	if !domain.Valid() {
		return nil, invalidRequest("unknown domain %d", domain)
	}
	var p bestSellersParams
	for _, o := range opts {
		o.applyBestSellers(&p)
	}
	switch p.rankRange {
	case 0, 30, 90, 180:
	default:
		return nil, invalidRequest("rank range must be 30, 90 or 180 days, got %d", p.rankRange)
	}

	q := c.query()
	q.Set("domain", domain.queryValue())
	q.Set("category", strconv.FormatInt(categoryID, 10))
	if p.rankRange != 0 {
		q.Set("range", strconv.Itoa(p.rankRange))
	}
	if p.variations {
		q.Set("variations", "1")
	}

	return do[BestSellersResponse](ctx, c, request{
		path:       "/bestsellers",
		query:      q,
		cost:       50,
		timeout:    2 * c.timeout,
		callParams: p.callParams,
	})
}

// BestSellersResponse is returned by GetBestSellers. Keepa documents the list
// under bestSellersList; production responses also carry it at the top level.
type BestSellersResponse struct {
	Envelope
	BestSellersList BestSellers `json:"bestSellersList"`
	ASINList        []string    `json:"asinList"`
}

// BestSellers is the documented best sellers object.
type BestSellers struct {
	DomainID   int      `json:"domainId"`
	LastUpdate Time     `json:"lastUpdate"`
	CategoryID int64    `json:"categoryId"`
	ASINList   []string `json:"asinList"`
}

// ASINs returns the top-level list when present, otherwise the nested one.
func (r *BestSellersResponse) ASINs() []string {
	if len(r.ASINList) > 0 {
		return r.ASINList
	}
	return r.BestSellersList.ASINList
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test -race -count=1 ./...`
Expected: `ok`

- [ ] **Step 6: Commit**

```bash
git -C /Users/jameseagle/code/Jleagle/keepa add bestsellers.go bestsellers_test.go testdata/bestsellers.json
git -C /Users/jameseagle/code/Jleagle/keepa commit -m "Add best sellers endpoint"
```

---

### Task 10: Categories endpoint

**Files:**
- Create: `category.go`, `testdata/categories.json`
- Test: `category_test.go`

**Interfaces:**
- Consumes: `do`, `request`, `categoryParams`, `categoryOption` (defined in `options.go`), `CategoryOption`, `Domain`, `invalidRequest`, test helpers.
- Produces: `func (c *Client) GetCategories(ctx context.Context, domain Domain, categoryIDs []int64, opts ...CategoryOption) (*CategoryResponse, error)`; option `WithParents()`; types `CategoryResponse`, `Category`.

- [ ] **Step 1: Write the fixture**

`testdata/categories.json`:

```json
{
  "timestamp": 1790277678673,
  "tokensLeft": 1199,
  "tokensConsumed": 1,
  "refillIn": 30000,
  "refillRate": 20,
  "tokenFlowReduction": 0,
  "processingTimeInMs": 3,
  "categories": {
    "172282": {
      "catId": 172282,
      "name": "Electronics",
      "parent": 0,
      "children": [502394],
      "domainId": 1,
      "productCount": 123456,
      "isBrowseNode": true,
      "contextFreeName": "Electronics",
      "websiteDisplayGroup": "ce_display_on_website",
      "avgRating": 42,
      "isFBAPercent": 55.5,
      "topBrands": ["Acme", "Globex"]
    },
    "502394": {
      "catId": 502394,
      "name": "Camera & Photo",
      "parent": 172282,
      "children": [],
      "domainId": 1
    }
  },
  "categoryParents": {}
}
```

- [ ] **Step 2: Write the failing tests**

`category_test.go`:

```go
package keepa

import (
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"testing"
)

func TestGetCategoriesRequestDefaults(t *testing.T) {
	c, rec := newTestClient(t, serveFixture(t, "categories.json"))
	if _, err := c.GetCategories(t.Context(), DomainDE, []int64{172282, 502394}); err != nil {
		t.Fatal(err)
	}
	last := rec.Last(t)
	if last.method != http.MethodGet || last.path != "/category" {
		t.Errorf("request = %s %s", last.method, last.path)
	}
	// Keepa lists parents as required, so 0 is sent explicitly.
	want := url.Values{"key": {"test-key"}, "domain": {"3"}, "category": {"172282,502394"}, "parents": {"0"}}
	if !maps.EqualFunc(last.query, want, slices.Equal) {
		t.Errorf("query = %v, want exactly %v", last.query, want)
	}
}

func TestGetCategoriesWithParents(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope(`"categories":{}`)))
	if _, err := c.GetCategories(t.Context(), DomainUS, []int64{1}, WithParents()); err != nil {
		t.Fatal(err)
	}
	if got := rec.Last(t).query.Get("parents"); got != "1" {
		t.Errorf("parents = %q, want 1", got)
	}
}

func TestGetCategoriesValidation(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	tests := []struct {
		name   string
		domain Domain
		ids    []int64
	}{
		{"no ids", DomainUS, nil},
		{"eleven ids", DomainUS, make([]int64, 11)},
		{"reserved domain", Domain(7), []int64{1}},
	}
	for _, tt := range tests {
		if _, err := c.GetCategories(t.Context(), tt.domain, tt.ids); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: err = %v, want ErrInvalidRequest", tt.name, err)
		}
	}
	if n := len(rec.Calls()); n != 0 {
		t.Errorf("%d requests sent before validation", n)
	}
	if _, err := c.GetCategories(t.Context(), DomainUS, make([]int64, 10)); err != nil {
		t.Errorf("10 ids must be allowed: %v", err)
	}
}

func TestGetCategoriesCost(t *testing.T) {
	c, _ := newTestClient(t, serveJSON(200, okEnvelope("")))
	_, err := c.GetCategories(t.Context(), DomainUS, []int64{1, 2, 3}, WithReserve(1200), WithoutWaiting())
	if got := costOf(t, err); got != 1 {
		t.Errorf("cost = %d, want 1 regardless of the number of ids", got)
	}
}

func TestGetCategoriesDecodes(t *testing.T) {
	c, _ := newTestClient(t, serveFixture(t, "categories.json"))
	res, err := c.GetCategories(t.Context(), DomainUS, []int64{172282, 502394})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Categories) != 2 {
		t.Fatalf("categories = %d, want 2", len(res.Categories))
	}
	root := res.Categories[172282]
	if root.Name != "Electronics" || root.Parent != 0 || !slices.Equal(root.Children, []int64{502394}) || root.ProductCount != 123456 || !root.IsBrowseNode {
		t.Errorf("root = %+v", root)
	}
	if root.AvgRating != 42 || root.IsFBAPercent != 55.5 || !slices.Equal(root.TopBrands, []string{"Acme", "Globex"}) {
		t.Errorf("root stats = %+v", root)
	}
	child := res.Categories[502394]
	if child.Name != "Camera & Photo" || child.Parent != 172282 || len(child.Children) != 0 {
		t.Errorf("child = %+v", child)
	}
	if len(res.CategoryParents) != 0 {
		t.Errorf("categoryParents = %v, want empty", res.CategoryParents)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./... 2>&1 | head -20`
Expected: build failure mentioning `GetCategories`.

- [ ] **Step 4: Write the implementation**

`category.go`:

```go
package keepa

import (
	"context"
	"strconv"
	"strings"
)

// WithParents includes each category's ancestors in CategoryParents.
func WithParents() CategoryOption {
	return categoryOption(func(p *categoryParams) { p.parents = true })
}

// GetCategories looks up between 1 and 10 categories by ID. Cost: 1 token.
func (c *Client) GetCategories(ctx context.Context, domain Domain, categoryIDs []int64, opts ...CategoryOption) (*CategoryResponse, error) {
	if !domain.Valid() {
		return nil, invalidRequest("unknown domain %d", domain)
	}
	if len(categoryIDs) == 0 || len(categoryIDs) > 10 {
		return nil, invalidRequest("between 1 and 10 category IDs required, got %d", len(categoryIDs))
	}
	var p categoryParams
	for _, o := range opts {
		o.applyCategory(&p)
	}

	ids := make([]string, 0, len(categoryIDs))
	for _, id := range categoryIDs {
		ids = append(ids, strconv.FormatInt(id, 10))
	}
	q := c.query()
	q.Set("domain", domain.queryValue())
	q.Set("category", strings.Join(ids, ","))
	// Keepa documents parents as required, so it is always sent.
	if p.parents {
		q.Set("parents", "1")
	} else {
		q.Set("parents", "0")
	}

	return do[CategoryResponse](ctx, c, request{
		path:       "/category",
		query:      q,
		cost:       1,
		callParams: p.callParams,
	})
}

// CategoryResponse is returned by GetCategories. Maps are keyed by category ID.
type CategoryResponse struct {
	Envelope
	Categories      map[int64]Category `json:"categories"`
	CategoryParents map[int64]Category `json:"categoryParents"`
}

// Category is a Keepa category object.
type Category struct {
	AvgBuyBox               int      `json:"avgBuyBox"`
	AvgBuyBox365            int      `json:"avgBuyBox365"`
	AvgBuyBox90             int      `json:"avgBuyBox90"`
	AvgDeltaPercent90Amazon float64  `json:"avgDeltaPercent90Amazon"`
	AvgDeltaPercent90BuyBox float64  `json:"avgDeltaPercent90BuyBox"`
	AvgOfferCountNew        float64  `json:"avgOfferCountNew"`
	AvgOfferCountUsed       float64  `json:"avgOfferCountUsed"`
	AvgRating               int      `json:"avgRating"`
	AvgReviewCount          int      `json:"avgReviewCount"`
	BrandCount              int      `json:"brandCount"`
	CatID                   int64    `json:"catId"`
	Children                []int64  `json:"children"`
	ContextFreeName         string   `json:"contextFreeName"`
	DomainID                int      `json:"domainId"`
	HighestRank             int      `json:"highestRank"`
	IsBrowseNode            bool     `json:"isBrowseNode"`
	IsFBAPercent            float64  `json:"isFBAPercent"`
	LowestRank              int      `json:"lowestRank"`
	Name                    string   `json:"name"`
	Parent                  int64    `json:"parent"`
	ProductCount            int      `json:"productCount"`
	RelatedSellerNames      []string `json:"relatedSellerNames"`
	RelatedSellerNamesAny   []string `json:"relatedSellerNamesAny"`
	SellerCount             int      `json:"sellerCount"`
	SoldByAmazonPercent     float64  `json:"soldByAmazonPercent"`
	TopBrands               []string `json:"topBrands"`
	TopSellers              []string `json:"topSellers"`
	TopSellersAny           []string `json:"topSellersAny"`
	WebsiteDisplayGroup     string   `json:"websiteDisplayGroup"`
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test -race -count=1 ./...`
Expected: `ok`

- [ ] **Step 6: Commit**

```bash
git -C /Users/jameseagle/code/Jleagle/keepa add category.go category_test.go testdata/categories.json
git -C /Users/jameseagle/code/Jleagle/keepa commit -m "Add categories endpoint"
```

---

### Task 11: Deals endpoint

**Files:**
- Create: `deals.go`, `testdata/deals.json`
- Test: `deals_test.go`

**Interfaces:**
- Consumes: `do`, `request`, `dealsParams`, `dealsOption` (defined in `options.go`), `DealsOption`, `Domain`, `CSVType`, `Time`, `invalidRequest`, test helpers.
- Produces: `func (c *Client) GetDeals(ctx context.Context, domain Domain, priceType CSVType, opts ...DealsOption) (*DealsResponse, error)`; enums `DealsDateRange` (`DealsDateRangeDay`, `Week`, `Month`, `Quarter`), `DealsSort` (`DealsSortAge`, `Delta`, `SalesRank`, `DeltaPercent`); options `WithDealsPage`, `WithDealsDateRange`, `WithDealsMinRating`, `WithDealsOutOfStock`, `WithDealsFilterErotic`, `WithDealsSort`, `WithDealsDeltaLastRange`; types `DealsResponse`, `DealsPage`, `Deal` (with `ImageName()`).

- [ ] **Step 1: Write the fixture**

`testdata/deals.json` (the image array spells "51a.jpg" in ASCII codes):

```json
{
  "timestamp": 1790277678673,
  "tokensLeft": 1195,
  "tokensConsumed": 5,
  "refillIn": 30000,
  "refillRate": 20,
  "tokenFlowReduction": 0,
  "processingTimeInMs": 80,
  "deals": {
    "dr": [
      {
        "asin": "B000000001",
        "parentAsin": "",
        "title": "Deal One",
        "rootCat": 172282,
        "categories": [172282, 502394],
        "image": [53, 49, 97, 46, 106, 112, 103],
        "current": [1899, 1899, -1, 1234],
        "currentSince": [7204320, 7204320, -1, 7204000],
        "deltaLast": [-100, -100, 0, 0],
        "delta": [[-100, -100, 0, 0], [-200, -200, 0, 0], [-300, -300, 0, 0], [-400, -400, 0, 0]],
        "deltaPercent": [[-5, -5, 0, 0], [-10, -10, 0, 0], [-14, -14, 0, 0], [-17, -17, 0, 0]],
        "avg": [[1999, 1999, -1, 1300], [2099, 2099, -1, 1300], [2199, 2199, -1, 1300], [2299, 2299, -1, 1300]],
        "lastUpdate": 7204320,
        "creationDate": 7204300,
        "lightningEnd": 0,
        "warehouseCondition": 0,
        "warehouseConditionComment": null
      }
    ],
    "categoryIds": [172282],
    "categoryNames": ["Electronics"],
    "categoryCount": [1]
  }
}
```

- [ ] **Step 2: Write the failing tests**

`deals_test.go`:

```go
package keepa

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"testing"
)

func decodeBody(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("body %s: %v", b, err)
	}
	return m
}

func TestGetDealsRequestDefaults(t *testing.T) {
	c, rec := newTestClient(t, serveFixture(t, "deals.json"))
	if _, err := c.GetDeals(t.Context(), DomainUS, CSVAmazon); err != nil {
		t.Fatal(err)
	}
	last := rec.Last(t)
	if last.method != http.MethodPost || last.path != "/deal" || last.contentType != "application/json" {
		t.Errorf("request = %s %s %s", last.method, last.path, last.contentType)
	}
	if last.query.Get("key") != "test-key" || len(last.query) != 1 {
		t.Errorf("query = %v, want only the key", last.query)
	}
	want := map[string]any{"domainId": 1.0, "priceTypes": []any{0.0}, "page": 0.0}
	if got := decodeBody(t, last.body); !reflect.DeepEqual(got, want) {
		t.Errorf("body = %v, want exactly %v", got, want)
	}
}

func TestGetDealsOptions(t *testing.T) {
	tests := []struct {
		name string
		opts []DealsOption
		want map[string]any
	}{
		{"page", []DealsOption{WithDealsPage(3)}, map[string]any{"page": 3.0}},
		{"date range", []DealsOption{WithDealsDateRange(DealsDateRangeMonth)}, map[string]any{"dateRange": 2.0}},
		{"min rating", []DealsOption{WithDealsMinRating(35)}, map[string]any{"minRating": 35.0}},
		{"out of stock", []DealsOption{WithDealsOutOfStock()}, map[string]any{"isOutOfStock": true}},
		{"filter erotic", []DealsOption{WithDealsFilterErotic()}, map[string]any{"filterErotic": true}},
		{"sort", []DealsOption{WithDealsSort(DealsSortSalesRank)}, map[string]any{"sortType": 3.0}},
		{"sort inverted", []DealsOption{WithDealsSort(-DealsSortSalesRank)}, map[string]any{"sortType": -3.0}},
		{"delta last range", []DealsOption{WithDealsDeltaLastRange(0, 500)}, map[string]any{"deltaLastRange": []any{0.0, 500.0}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := newTestClient(t, serveJSON(200, okEnvelope(`"deals":{"dr":[]}`)))
			if _, err := c.GetDeals(t.Context(), DomainGB, CSVNew, tt.opts...); err != nil {
				t.Fatal(err)
			}
			got := decodeBody(t, rec.Last(t).body)
			want := map[string]any{"domainId": 2.0, "priceTypes": []any{1.0}, "page": 0.0}
			for k, v := range tt.want {
				want[k] = v
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("body = %v, want %v", got, want)
			}
		})
	}
}

func TestGetDealsValidation(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	tests := []struct {
		name      string
		domain    Domain
		priceType CSVType
		opts      []DealsOption
	}{
		{"reserved domain", Domain(7), CSVAmazon, nil},
		{"price type too high", DomainUS, CSVCountNewFBA, nil},
		{"negative price type", DomainUS, CSVType(-1), nil},
		{"negative page", DomainUS, CSVAmazon, []DealsOption{WithDealsPage(-1)}},
		{"date range out of range", DomainUS, CSVAmazon, []DealsOption{WithDealsDateRange(DealsDateRange(4))}},
		{"rating too high", DomainUS, CSVAmazon, []DealsOption{WithDealsMinRating(51)}},
		{"rating negative", DomainUS, CSVAmazon, []DealsOption{WithDealsMinRating(-1)}},
		{"sort zero", DomainUS, CSVAmazon, []DealsOption{WithDealsSort(0)}},
		{"sort too high", DomainUS, CSVAmazon, []DealsOption{WithDealsSort(5)}},
		{"delta range reversed", DomainUS, CSVAmazon, []DealsOption{WithDealsDeltaLastRange(10, 5)}},
	}
	for _, tt := range tests {
		if _, err := c.GetDeals(t.Context(), tt.domain, tt.priceType, tt.opts...); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: err = %v, want ErrInvalidRequest", tt.name, err)
		}
	}
	if n := len(rec.Calls()); n != 0 {
		t.Errorf("%d requests sent before validation", n)
	}
	if _, err := c.GetDeals(t.Context(), DomainUS, CSVPrimeExclusive, WithDealsMinRating(0), WithDealsSort(-DealsSortDeltaPercent)); err != nil {
		t.Errorf("boundary values must be allowed: %v", err)
	}
}

func TestGetDealsCost(t *testing.T) {
	c, _ := newTestClient(t, serveJSON(200, okEnvelope("")))
	_, err := c.GetDeals(t.Context(), DomainUS, CSVAmazon, WithReserve(1200), WithoutWaiting())
	if got := costOf(t, err); got != 5 {
		t.Errorf("cost = %d, want 5", got)
	}
}

func TestGetDealsDecodes(t *testing.T) {
	c, _ := newTestClient(t, serveFixture(t, "deals.json"))
	res, err := c.GetDeals(t.Context(), DomainUS, CSVAmazon)
	if err != nil {
		t.Fatal(err)
	}
	page := res.Deals
	if len(page.Deals) != 1 || !slices.Equal(page.CategoryIDs, []int64{172282}) || !slices.Equal(page.CategoryNames, []string{"Electronics"}) || !slices.Equal(page.CategoryCount, []int{1}) {
		t.Errorf("page = %+v", page)
	}
	d := page.Deals[0]
	if d.ASIN != "B000000001" || d.Title != "Deal One" || d.RootCategory != 172282 || !slices.Equal(d.Categories, []int64{172282, 502394}) {
		t.Errorf("deal = %+v", d)
	}
	if got := d.ImageName(); got != "51a.jpg" {
		t.Errorf("ImageName() = %q, want 51a.jpg", got)
	}
	if d.Current[CSVAmazon] != 1899 || d.CurrentSince[CSVAmazon] != 7204320 || d.DeltaLast[CSVAmazon] != -100 {
		t.Errorf("current/deltaLast = %v %v %v", d.Current, d.CurrentSince, d.DeltaLast)
	}
	if d.Delta[0][CSVAmazon] != -100 || d.Delta[3][CSVNew] != -400 || d.DeltaPercent[1][CSVAmazon] != -10 || d.Avg[2][CSVSales] != 1300 {
		t.Errorf("delta/avg = %v %v %v", d.Delta, d.DeltaPercent, d.Avg)
	}
	if d.LastUpdate != 7204320 || d.CreationDate != 7204300 || d.LightningEnd.Valid() {
		t.Errorf("times = %d %d %d", d.LastUpdate, d.CreationDate, d.LightningEnd)
	}
	if d.WarehouseCondition != 0 || d.WarehouseConditionComment != "" {
		t.Errorf("warehouse = %d %q", d.WarehouseCondition, d.WarehouseConditionComment)
	}
	if got := (Deal{}).ImageName(); got != "" {
		t.Errorf("ImageName() on empty = %q", got)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./... 2>&1 | head -20`
Expected: build failure mentioning `GetDeals`.

- [ ] **Step 4: Write the implementation**

`deals.go`:

```go
package keepa

import (
	"context"
	"strings"
)

// DealsDateRange is the interval deals are computed over.
type DealsDateRange int

const (
	DealsDateRangeDay     DealsDateRange = 0
	DealsDateRangeWeek    DealsDateRange = 1
	DealsDateRangeMonth   DealsDateRange = 2
	DealsDateRangeQuarter DealsDateRange = 3 // 90 days
)

// DealsSort orders the deals. Negate a value to invert the order, for
// example -DealsSortSalesRank.
type DealsSort int

const (
	DealsSortAge          DealsSort = 1
	DealsSortDelta        DealsSort = 2
	DealsSortSalesRank    DealsSort = 3
	DealsSortDeltaPercent DealsSort = 4
)

// WithDealsPage selects a page of up to 150 deals, starting at 0.
func WithDealsPage(n int) DealsOption {
	return dealsOption(func(p *dealsParams) { p.page = n })
}

// WithDealsDateRange sets the interval deals are computed over. Keepa's
// default is the last day.
func WithDealsDateRange(r DealsDateRange) DealsOption {
	return dealsOption(func(p *dealsParams) { v := int(r); p.dateRange = &v })
}

// WithDealsMinRating requires a product rating of at least r on Keepa's
// 0 to 50 scale, so 35 means 3.5 stars.
func WithDealsMinRating(r int) DealsOption {
	return dealsOption(func(p *dealsParams) { p.minRating = &r })
}

// WithDealsOutOfStock returns products that went out of stock within the
// interval instead of price drops.
func WithDealsOutOfStock() DealsOption {
	return dealsOption(func(p *dealsParams) { p.outOfStock = true })
}

// WithDealsFilterErotic excludes adult products.
func WithDealsFilterErotic() DealsOption {
	return dealsOption(func(p *dealsParams) { p.filterErotic = true })
}

// WithDealsSort orders the results.
func WithDealsSort(s DealsSort) DealsOption {
	return dealsOption(func(p *dealsParams) { v := int(s); p.sort = &v })
}

// WithDealsDeltaLastRange keeps deals whose last price change, in the
// smallest currency unit, lies between min and max.
func WithDealsDeltaLastRange(min, max int) DealsOption {
	return dealsOption(func(p *dealsParams) { p.deltaLast = &[2]int{min, max} })
}

// GetDeals browses recent price changes for one price type. Keepa requires
// exactly one price type per query. Cost: 5 tokens per page of up to 150 deals.
func (c *Client) GetDeals(ctx context.Context, domain Domain, priceType CSVType, opts ...DealsOption) (*DealsResponse, error) {
	if !domain.Valid() {
		return nil, invalidRequest("unknown domain %d", domain)
	}
	if priceType < CSVAmazon || priceType > CSVPrimeExclusive {
		return nil, invalidRequest("price type must be between 0 and 33, got %d", priceType)
	}
	var p dealsParams
	for _, o := range opts {
		o.applyDeals(&p)
	}
	if p.page < 0 {
		return nil, invalidRequest("page must not be negative, got %d", p.page)
	}
	if p.dateRange != nil && (*p.dateRange < 0 || *p.dateRange > 3) {
		return nil, invalidRequest("date range must be between 0 and 3, got %d", *p.dateRange)
	}
	if p.minRating != nil && (*p.minRating < 0 || *p.minRating > 50) {
		return nil, invalidRequest("minimum rating must be between 0 and 50, got %d", *p.minRating)
	}
	if p.sort != nil && (abs(*p.sort) < 1 || abs(*p.sort) > 4) {
		return nil, invalidRequest("sort must be between 1 and 4 or its negative, got %d", *p.sort)
	}
	if p.deltaLast != nil && p.deltaLast[0] > p.deltaLast[1] {
		return nil, invalidRequest("delta last range minimum %d exceeds maximum %d", p.deltaLast[0], p.deltaLast[1])
	}

	body := map[string]any{
		"domainId":   int(domain),
		"priceTypes": []int{int(priceType)},
		"page":       p.page,
	}
	if p.dateRange != nil {
		body["dateRange"] = *p.dateRange
	}
	if p.minRating != nil {
		body["minRating"] = *p.minRating
	}
	if p.outOfStock {
		body["isOutOfStock"] = true
	}
	if p.filterErotic {
		body["filterErotic"] = true
	}
	if p.sort != nil {
		body["sortType"] = *p.sort
	}
	if p.deltaLast != nil {
		body["deltaLastRange"] = []int{p.deltaLast[0], p.deltaLast[1]}
	}

	return do[DealsResponse](ctx, c, request{
		path:       "/deal",
		query:      c.query(),
		body:       body,
		cost:       5,
		callParams: p.callParams,
	})
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// DealsResponse is returned by GetDeals.
type DealsResponse struct {
	Envelope
	Deals DealsPage `json:"deals"`
}

// DealsPage is one page of deals with the root category breakdown.
type DealsPage struct {
	Deals         []Deal   `json:"dr"`
	CategoryIDs   []int64  `json:"categoryIds"`
	CategoryNames []string `json:"categoryNames"`
	CategoryCount []int    `json:"categoryCount"`
}

// Deal is a Keepa deal object. Per-price-type arrays are indexed by CSVType;
// two-dimensional arrays are indexed by date range (0 = 48 hours, 1 = week,
// 2 = month, 3 = 90 days) then price type.
type Deal struct {
	ASIN                      string  `json:"asin"`
	ParentASIN                string  `json:"parentAsin"`
	Title                     string  `json:"title"`
	RootCategory              int64   `json:"rootCat"`
	Categories                []int64 `json:"categories"`
	Image                     []int   `json:"image"` // ASCII codes of the file name; see ImageName
	Current                   []int   `json:"current"`
	CurrentSince              []Time  `json:"currentSince"`
	DeltaLast                 []int   `json:"deltaLast"`
	Delta                     [][]int `json:"delta"`
	DeltaPercent              [][]int `json:"deltaPercent"`
	Avg                       [][]int `json:"avg"`
	LastUpdate                Time    `json:"lastUpdate"`
	CreationDate              Time    `json:"creationDate"`
	LightningEnd              Time    `json:"lightningEnd"`
	WarehouseCondition        int     `json:"warehouseCondition"`
	WarehouseConditionComment string  `json:"warehouseConditionComment"`
}

// ImageName decodes the image file name Keepa sends as ASCII codes.
func (d Deal) ImageName() string {
	var b strings.Builder
	for _, code := range d.Image {
		b.WriteRune(rune(code))
	}
	return b.String()
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test -race -count=1 ./...`
Expected: `ok`

- [ ] **Step 6: Commit**

```bash
git -C /Users/jameseagle/code/Jleagle/keepa add deals.go deals_test.go testdata/deals.json
git -C /Users/jameseagle/code/Jleagle/keepa commit -m "Add deals endpoint"
```

---

### Task 12: Lightning deals endpoint

**Files:**
- Create: `lightning_deals.go`, `testdata/lightning_deals.json`
- Test: `lightning_deals_test.go`

**Interfaces:**
- Consumes: `do`, `request`, `lightningDealsParams`, `LightningDealsOption`, `Domain`, `Time`, `VariationAttribute`, `invalidRequest`, test helpers.
- Produces: `func (c *Client) GetLightningDeals(ctx context.Context, domain Domain, opts ...LightningDealsOption) (*LightningDealsResponse, error)`; types `LightningDealsResponse`, `LightningDeal`.

- [ ] **Step 1: Write the fixture**

`testdata/lightning_deals.json`:

```json
{
  "timestamp": 1790277678673,
  "tokensLeft": 700,
  "tokensConsumed": 500,
  "refillIn": 30000,
  "refillRate": 20,
  "tokenFlowReduction": 0,
  "processingTimeInMs": 120,
  "lightningDeals": [
    {
      "domainId": 1,
      "lastUpdate": 7204320,
      "asin": "B000000001",
      "title": "Lightning One",
      "sellerId": "ATVPDKIKX0DER",
      "sellerName": "Amazon.com",
      "dealId": "abc123",
      "dealPrice": 1499,
      "currentPrice": 1999,
      "image": "51abc.jpg",
      "isPrimeEligible": true,
      "isFulfilledByAmazon": true,
      "rating": 45,
      "totalReviews": 1200,
      "dealState": "AVAILABLE",
      "startTime": 7204000,
      "endTime": 7204600,
      "percentClaimed": 40,
      "percentOff": 25,
      "variation": [{"dimension": "Size", "value": "L"}]
    },
    {
      "domainId": 1,
      "lastUpdate": 7204320,
      "asin": "B000000002",
      "title": "Upcoming",
      "dealPrice": -1,
      "currentPrice": 999,
      "dealState": "WAITLIST",
      "startTime": 7205000,
      "endTime": 7205600
    }
  ]
}
```

- [ ] **Step 2: Write the failing tests**

`lightning_deals_test.go`:

```go
package keepa

import (
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"testing"
)

func TestGetLightningDealsRequest(t *testing.T) {
	c, rec := newTestClient(t, serveFixture(t, "lightning_deals.json"))
	if _, err := c.GetLightningDeals(t.Context(), DomainJP); err != nil {
		t.Fatal(err)
	}
	last := rec.Last(t)
	if last.method != http.MethodGet || last.path != "/lightningdeal" {
		t.Errorf("request = %s %s", last.method, last.path)
	}
	// An empty asin parameter makes Keepa answer 400, so nothing but key and domain may be sent.
	want := url.Values{"key": {"test-key"}, "domain": {"5"}}
	if !maps.EqualFunc(last.query, want, slices.Equal) {
		t.Errorf("query = %v, want exactly %v", last.query, want)
	}
}

func TestGetLightningDealsValidationAndCost(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	if _, err := c.GetLightningDeals(t.Context(), Domain(7)); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("reserved domain: err = %v", err)
	}
	if n := len(rec.Calls()); n != 0 {
		t.Errorf("%d requests sent before validation", n)
	}
	_, err := c.GetLightningDeals(t.Context(), DomainUS, WithReserve(1200), WithoutWaiting())
	if got := costOf(t, err); got != 500 {
		t.Errorf("cost = %d, want 500", got)
	}
}

func TestGetLightningDealsDecodes(t *testing.T) {
	c, _ := newTestClient(t, serveFixture(t, "lightning_deals.json"))
	res, err := c.GetLightningDeals(t.Context(), DomainUS)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.LightningDeals) != 2 || res.TokensConsumed != 500 {
		t.Fatalf("response = %d deals, %d consumed", len(res.LightningDeals), res.TokensConsumed)
	}
	d := res.LightningDeals[0]
	if d.ASIN != "B000000001" || d.Title != "Lightning One" || d.SellerID != "ATVPDKIKX0DER" || d.DealID != "abc123" {
		t.Errorf("identity = %+v", d)
	}
	if d.DealPrice != 1499 || d.CurrentPrice != 1999 || d.PercentOff != 25 || d.PercentClaimed != 40 {
		t.Errorf("prices = %+v", d)
	}
	if d.StartTime != 7204000 || d.EndTime != 7204600 || d.LastUpdate != 7204320 || !d.EndTime.Valid() {
		t.Errorf("times = %+v", d)
	}
	if d.DealState != "AVAILABLE" || !d.IsPrimeEligible || !d.IsFulfilledByAmazon || d.Rating != 45 || d.TotalReviews != 1200 || d.Image != "51abc.jpg" {
		t.Errorf("flags = %+v", d)
	}
	if len(d.Variation) != 1 || d.Variation[0] != (VariationAttribute{Dimension: "Size", Value: "L"}) {
		t.Errorf("variation = %+v", d.Variation)
	}
	upcoming := res.LightningDeals[1]
	if upcoming.DealPrice != -1 || upcoming.DealState != "WAITLIST" || upcoming.Variation != nil {
		t.Errorf("upcoming = %+v", upcoming)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./... 2>&1 | head -20`
Expected: build failure mentioning `GetLightningDeals`.

- [ ] **Step 4: Write the implementation**

`lightning_deals.go`:

```go
package keepa

import "context"

// GetLightningDeals retrieves every lightning deal of the last four days,
// active and expired. Cost: 500 tokens.
func (c *Client) GetLightningDeals(ctx context.Context, domain Domain, opts ...LightningDealsOption) (*LightningDealsResponse, error) {
	if !domain.Valid() {
		return nil, invalidRequest("unknown domain %d", domain)
	}
	var p lightningDealsParams
	for _, o := range opts {
		o.applyLightningDeals(&p)
	}

	q := c.query()
	q.Set("domain", domain.queryValue())

	return do[LightningDealsResponse](ctx, c, request{
		path:       "/lightningdeal",
		query:      q,
		cost:       500,
		callParams: p.callParams,
	})
}

// LightningDealsResponse is returned by GetLightningDeals.
type LightningDealsResponse struct {
	Envelope
	LightningDeals []LightningDeal `json:"lightningDeals"`
}

// LightningDeal is a Keepa lightning deal object. Prices are in the smallest
// currency unit; DealPrice is -1 while the deal is upcoming.
type LightningDeal struct {
	DomainID            int                  `json:"domainId"`
	LastUpdate          Time                 `json:"lastUpdate"`
	ASIN                string               `json:"asin"`
	Title               string               `json:"title"`
	SellerID            string               `json:"sellerId"`
	SellerName          string               `json:"sellerName"`
	DealID              string               `json:"dealId"`
	DealPrice           int                  `json:"dealPrice"`
	CurrentPrice        int                  `json:"currentPrice"`
	Image               string               `json:"image"`
	IsPrimeEligible     bool                 `json:"isPrimeEligible"`
	IsFulfilledByAmazon bool                 `json:"isFulfilledByAmazon"`
	Rating              int                  `json:"rating"` // 0 to 50
	TotalReviews        int                  `json:"totalReviews"`
	DealState           string               `json:"dealState"` // AVAILABLE, WAITLIST, SOLDOUT, WAITLISTFULL, EXPIRED, SUPPRESSED
	StartTime           Time                 `json:"startTime"`
	EndTime             Time                 `json:"endTime"`
	PercentClaimed      int                  `json:"percentClaimed"`
	PercentOff          int                  `json:"percentOff"`
	Variation           []VariationAttribute `json:"variation"`
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test -race -count=1 ./...`
Expected: `ok`

- [ ] **Step 6: Commit**

```bash
git -C /Users/jameseagle/code/Jleagle/keepa add lightning_deals.go lightning_deals_test.go testdata/lightning_deals.json
git -C /Users/jameseagle/code/Jleagle/keepa commit -m "Add lightning deals endpoint"
```

---

### Task 13: Search endpoint

**Files:**
- Create: `search.go`, `testdata/search.json`
- Test: `search_test.go`

**Interfaces:**
- Consumes: `do`, `request`, `searchParams`, `searchOption` (defined in `options.go`), `SearchOption`, `Domain`, `Product`, `invalidRequest`, test helpers.
- Produces: `func (c *Client) SearchProducts(ctx context.Context, domain Domain, term string, opts ...SearchOption) (*SearchResponse, error)`; option `WithASINsOnly()`; type `SearchResponse`.

- [ ] **Step 1: Write the fixture**

`testdata/search.json`:

```json
{
  "timestamp": 1790277678673,
  "tokensLeft": 1190,
  "tokensConsumed": 10,
  "refillIn": 30000,
  "refillRate": 20,
  "tokenFlowReduction": 0,
  "processingTimeInMs": 60,
  "products": [
    {"asin": "B000000001", "title": "Found One", "domainId": 1, "lastUpdate": 7204320}
  ],
  "asinList": ["B000000001", "B000000002"]
}
```

- [ ] **Step 2: Write the failing tests**

`search_test.go`:

```go
package keepa

import (
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"testing"
)

func TestSearchProductsRequestDefaults(t *testing.T) {
	c, rec := newTestClient(t, serveFixture(t, "search.json"))
	if _, err := c.SearchProducts(t.Context(), DomainGB, "usb c cable & charger"); err != nil {
		t.Fatal(err)
	}
	last := rec.Last(t)
	if last.method != http.MethodGet || last.path != "/search" {
		t.Errorf("request = %s %s", last.method, last.path)
	}
	want := url.Values{"key": {"test-key"}, "domain": {"2"}, "type": {"product"}, "term": {"usb c cable & charger"}}
	if !maps.EqualFunc(last.query, want, slices.Equal) {
		t.Errorf("query = %v, want exactly %v", last.query, want)
	}
}

func TestSearchProductsASINsOnly(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope(`"asinList":[]`)))
	if _, err := c.SearchProducts(t.Context(), DomainUS, "cable", WithASINsOnly()); err != nil {
		t.Fatal(err)
	}
	if got := rec.Last(t).query.Get("asins-only"); got != "1" {
		t.Errorf("asins-only = %q, want 1", got)
	}
}

func TestSearchProductsValidationAndCost(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	for _, term := range []string{"", "   "} {
		if _, err := c.SearchProducts(t.Context(), DomainUS, term); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("term %q: err = %v, want ErrInvalidRequest", term, err)
		}
	}
	if _, err := c.SearchProducts(t.Context(), Domain(7), "cable"); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("reserved domain: err = %v", err)
	}
	if n := len(rec.Calls()); n != 0 {
		t.Errorf("%d requests sent before validation", n)
	}
	_, err := c.SearchProducts(t.Context(), DomainUS, "cable", WithReserve(1200), WithoutWaiting())
	if got := costOf(t, err); got != 10 {
		t.Errorf("cost = %d, want 10", got)
	}
}

func TestSearchProductsDecodes(t *testing.T) {
	c, _ := newTestClient(t, serveFixture(t, "search.json"))
	res, err := c.SearchProducts(t.Context(), DomainUS, "cable")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Products) != 1 || res.Products[0].ASIN != "B000000001" || res.Products[0].Title != "Found One" || res.Products[0].LastUpdate != 7204320 {
		t.Errorf("products = %+v", res.Products)
	}
	if !slices.Equal(res.ASINList, []string{"B000000001", "B000000002"}) {
		t.Errorf("asinList = %v", res.ASINList)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./... 2>&1 | head -20`
Expected: build failure mentioning `SearchProducts`.

- [ ] **Step 4: Write the implementation**

`search.go`:

```go
package keepa

import (
	"context"
	"strings"
)

// WithASINsOnly returns only the ASINList instead of full product objects.
func WithASINsOnly() SearchOption {
	return searchOption(func(p *searchParams) { p.asinsOnly = true })
}

// SearchProducts searches a marketplace by keyword and returns up to 20
// products. Cost: 10 tokens.
func (c *Client) SearchProducts(ctx context.Context, domain Domain, term string, opts ...SearchOption) (*SearchResponse, error) {
	if !domain.Valid() {
		return nil, invalidRequest("unknown domain %d", domain)
	}
	if strings.TrimSpace(term) == "" {
		return nil, invalidRequest("search term required")
	}
	var p searchParams
	for _, o := range opts {
		o.applySearch(&p)
	}

	q := c.query()
	q.Set("domain", domain.queryValue())
	q.Set("type", "product")
	q.Set("term", term)
	if p.asinsOnly {
		q.Set("asins-only", "1")
	}

	return do[SearchResponse](ctx, c, request{
		path:       "/search",
		query:      q,
		cost:       10,
		callParams: p.callParams,
	})
}

// SearchResponse is returned by SearchProducts. Products is empty when
// WithASINsOnly was used.
type SearchResponse struct {
	Envelope
	Products []Product `json:"products"`
	ASINList []string  `json:"asinList"`
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test -race -count=1 ./...`
Expected: `ok`

- [ ] **Step 6: Commit**

```bash
git -C /Users/jameseagle/code/Jleagle/keepa add search.go search_test.go testdata/search.json
git -C /Users/jameseagle/code/Jleagle/keepa commit -m "Add product search endpoint"
```

---

### Task 14: Sellers endpoint

**Files:**
- Create: `seller.go`, `testdata/sellers.json`
- Test: `seller_test.go`

**Interfaces:**
- Consumes: `do`, `request`, `sellersParams`, `SellersOption`, `Domain`, `Time`, `invalidRequest`, test helpers.
- Produces: `func (c *Client) GetSellers(ctx context.Context, domain Domain, sellerIDs []string, opts ...SellersOption) (*SellersResponse, error)`; types `SellersResponse`, `Seller`.

- [ ] **Step 1: Write the fixture**

`testdata/sellers.json`:

```json
{
  "timestamp": 1790277678673,
  "tokensLeft": 1199,
  "tokensConsumed": 1,
  "refillIn": 30000,
  "refillRate": 20,
  "tokenFlowReduction": 0,
  "processingTimeInMs": 8,
  "sellers": {
    "A2L77EE7U53NWQ": {
      "sellerId": "A2L77EE7U53NWQ",
      "sellerName": "Amazon Warehouse",
      "domainId": 1,
      "trackedSince": 4000000,
      "lastUpdate": 7204320,
      "hasFBA": true,
      "isScammer": false,
      "shipsFromChina": false,
      "currentRating": 95,
      "currentRatingCount": 10000,
      "ratingsLast30Days": 120,
      "csv": [[7200000, 94, 7204320, 95], [7200000, 9900, 7204320, 10000]],
      "totalStorefrontAsins": [7204320, 1200],
      "asinList": ["B000000001"],
      "asinListLastSeen": [7204320],
      "address": ["1 Example Street", "US"],
      "sellerBrandStatistics": null,
      "sellerCategoryStatistics": null
    }
  }
}
```

- [ ] **Step 2: Write the failing tests**

`seller_test.go`:

```go
package keepa

import (
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"testing"
)

func TestGetSellersRequestDefaults(t *testing.T) {
	c, rec := newTestClient(t, serveFixture(t, "sellers.json"))
	if _, err := c.GetSellers(t.Context(), DomainCA, []string{"A2L77EE7U53NWQ", "ATVPDKIKX0DER"}); err != nil {
		t.Fatal(err)
	}
	last := rec.Last(t)
	if last.method != http.MethodGet || last.path != "/seller" {
		t.Errorf("request = %s %s", last.method, last.path)
	}
	want := url.Values{"key": {"test-key"}, "domain": {"6"}, "seller": {"A2L77EE7U53NWQ,ATVPDKIKX0DER"}}
	if !maps.EqualFunc(last.query, want, slices.Equal) {
		t.Errorf("query = %v, want exactly %v", last.query, want)
	}
}

func TestGetSellersValidation(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	tests := []struct {
		name   string
		domain Domain
		ids    []string
	}{
		{"no ids", DomainUS, nil},
		{"too many", DomainUS, make([]string, 101)},
		{"reserved domain", Domain(7), []string{"A"}},
	}
	for _, tt := range tests {
		if _, err := c.GetSellers(t.Context(), tt.domain, tt.ids); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: err = %v, want ErrInvalidRequest", tt.name, err)
		}
	}
	if n := len(rec.Calls()); n != 0 {
		t.Errorf("%d requests sent before validation", n)
	}
	if _, err := c.GetSellers(t.Context(), DomainUS, make([]string, 100)); err != nil {
		t.Errorf("100 ids must be allowed: %v", err)
	}
}

func TestGetSellersCost(t *testing.T) {
	c, _ := newTestClient(t, serveJSON(200, okEnvelope("")))
	_, err := c.GetSellers(t.Context(), DomainUS, []string{"A", "B", "C", "D"}, WithReserve(1200), WithoutWaiting())
	if got := costOf(t, err); got != 4 {
		t.Errorf("cost = %d, want 1 per seller", got)
	}
}

func TestGetSellersDecodes(t *testing.T) {
	c, _ := newTestClient(t, serveFixture(t, "sellers.json"))
	res, err := c.GetSellers(t.Context(), DomainUS, []string{"A2L77EE7U53NWQ"})
	if err != nil {
		t.Fatal(err)
	}
	s, ok := res.Sellers["A2L77EE7U53NWQ"]
	if !ok {
		t.Fatalf("seller missing: %v", res.Sellers)
	}
	if s.SellerID != "A2L77EE7U53NWQ" || s.SellerName != "Amazon Warehouse" || s.DomainID != 1 || !s.HasFBA || s.IsScammer || s.ShipsFromChina {
		t.Errorf("identity = %+v", s)
	}
	if s.TrackedSince != 4000000 || s.LastUpdate != 7204320 || !slices.Equal(s.ASINListLastSeen, []Time{7204320}) {
		t.Errorf("times = %+v", s)
	}
	if s.CurrentRating != 95 || s.CurrentRatingCount != 10000 || s.RatingsLast30Days != 120 {
		t.Errorf("ratings = %+v", s)
	}
	if len(s.CSV) != 2 || !slices.Equal(s.CSV[1], []int{7200000, 9900, 7204320, 10000}) {
		t.Errorf("csv = %v", s.CSV)
	}
	if !slices.Equal(s.TotalStorefrontAsins, []int{7204320, 1200}) || !slices.Equal(s.ASINList, []string{"B000000001"}) || !slices.Equal(s.Address, []string{"1 Example Street", "US"}) {
		t.Errorf("storefront = %+v", s)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./... 2>&1 | head -20`
Expected: build failure mentioning `GetSellers`.

- [ ] **Step 4: Write the implementation**

`seller.go`:

```go
package keepa

import (
	"context"
	"strings"
)

// GetSellers retrieves between 1 and 100 marketplace sellers by ID.
// Cost: 1 token per seller. Keepa charges nothing for sellers it does not know.
func (c *Client) GetSellers(ctx context.Context, domain Domain, sellerIDs []string, opts ...SellersOption) (*SellersResponse, error) {
	if !domain.Valid() {
		return nil, invalidRequest("unknown domain %d", domain)
	}
	if len(sellerIDs) == 0 || len(sellerIDs) > 100 {
		return nil, invalidRequest("between 1 and 100 seller IDs required, got %d", len(sellerIDs))
	}
	var p sellersParams
	for _, o := range opts {
		o.applySellers(&p)
	}

	q := c.query()
	q.Set("domain", domain.queryValue())
	q.Set("seller", strings.Join(sellerIDs, ","))

	return do[SellersResponse](ctx, c, request{
		path:       "/seller",
		query:      q,
		cost:       len(sellerIDs),
		callParams: p.callParams,
	})
}

// SellersResponse is returned by GetSellers, keyed by seller ID.
type SellersResponse struct {
	Envelope
	Sellers map[string]Seller `json:"sellers"`
}

// Seller is a Keepa marketplace seller object.
type Seller struct {
	Address                  []string `json:"address"`                  // business address lines, country code last
	ASINList                 []string `json:"asinList"`                 // storefront ASINs, when requested
	ASINListLastSeen         []Time   `json:"asinListLastSeen"`         // parallel to ASINList
	CSV                      [][]int  `json:"csv"`                      // 0: rating percentage history, 1: rating count history
	CurrentRating            int      `json:"currentRating"`            // percentage
	CurrentRatingCount       int      `json:"currentRatingCount"`       // lifetime ratings
	DomainID                 int      `json:"domainId"`
	HasFBA                   bool     `json:"hasFBA"`
	IsScammer                bool     `json:"isScammer"`
	LastUpdate               Time     `json:"lastUpdate"`
	RatingsLast30Days        int      `json:"ratingsLast30Days"`
	SellerBrandStatistics    any      `json:"sellerBrandStatistics"`
	SellerCategoryStatistics any      `json:"sellerCategoryStatistics"`
	SellerID                 string   `json:"sellerId"`
	SellerName               string   `json:"sellerName"`
	ShipsFromChina           bool     `json:"shipsFromChina"`
	TotalStorefrontAsins     []int    `json:"totalStorefrontAsins"` // [Keepa minutes, count]
	TrackedSince             Time     `json:"trackedSince"`
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `gofmt -l . && go vet ./... && go test -race -count=1 ./...`
Expected: `ok`

- [ ] **Step 6: Commit**

```bash
git -C /Users/jameseagle/code/Jleagle/keepa add seller.go seller_test.go testdata/sellers.json
git -C /Users/jameseagle/code/Jleagle/keepa commit -m "Add sellers endpoint"
```

---

### Task 15: README, CI workflow, compiled example and final verification

**Files:**
- Create: `README.md`, `.github/workflows/test.yml`, `example_test.go`

**Interfaces:**
- Consumes: the whole public API.
- Produces: a compiled (not executed) example that keeps the README's quick start honest.

- [ ] **Step 1: Write the compiled example**

`example_test.go` (an `Example` without an `Output:` comment is compiled by `go test` and `go vet` but never run, so it cannot reach the network):

```go
package keepa_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Jleagle/keepa"
)

func ExampleNewClient() {
	client := keepa.NewClient("your-api-key",
		keepa.WithTokenReserve(1000),
		keepa.WithTokenCallback(func(u keepa.TokenUpdate) {
			log.Printf("%s consumed %d tokens, %d left", u.Path, u.Consumed, u.Left)
		}),
	)

	ctx := context.Background()
	resp, err := client.GetProducts(ctx, keepa.DomainUS, []string{"B07XJ8C8F5"},
		keepa.WithStats(time.Now().AddDate(0, -1, 0)),
		keepa.WithRatings(),
	)
	if err != nil {
		var apiErr *keepa.APIError
		switch {
		case errors.Is(err, keepa.ErrNotEnoughTokens):
			log.Print("quota exhausted")
		case errors.As(err, &apiErr):
			log.Printf("keepa said %s: %s", apiErr.Type, apiErr.Message)
		default:
			log.Print(err)
		}
		return
	}

	for _, p := range resp.Products {
		fmt.Println(p.ASIN, p.Title, p.LastUpdate.Time())
		for at, price := range p.CSV.Amazon.Pairs() {
			if price >= 0 {
				fmt.Println(at.Time(), price)
			}
		}
	}
}

func ExampleWithoutWaiting() {
	client := keepa.NewClient("your-api-key", keepa.WithTokenReserve(1000))

	// A queue consumer that would rather re-queue than block.
	_, err := client.GetLightningDeals(context.Background(), keepa.DomainUS, keepa.WithoutWaiting())
	var wait *keepa.TokenWaitError
	if errors.As(err, &wait) {
		fmt.Println("retry in", wait.Wait)
	}

	// An interactive call that may dip below the floor.
	_, _ = client.SearchProducts(context.Background(), keepa.DomainUS, "usb c cable",
		keepa.WithASINsOnly(), keepa.WithReserve(0))
}
```

- [ ] **Step 2: Write the CI workflow**

`.github/workflows/test.yml`:

```yaml
name: Test

on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7

      - uses: actions/setup-go@v7
        with:
          go-version-file: go.mod

      - name: Check formatting
        run: |
          if [ -n "$(gofmt -l .)" ]; then
            gofmt -d .
            exit 1
          fi

      - name: Vet
        run: go vet ./...

      - name: Test
        run: go test -race -count=1 ./...
```

- [ ] **Step 3: Write the README**

`README.md`:

````markdown
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

Options default to Keepa's own defaults, meaning the parameter is not sent.

## Client options

| Option | Default |
|---|---|
| `WithHTTPClient(*http.Client)` | 30 second timeout |
| `WithBaseURL(string)` | `https://api.keepa.com` |
| `WithLimiter(Limiter)` | none; `*rate.Limiter` from `golang.org/x/time/rate` fits |
| `WithLogger(*slog.Logger)` | discard |
| `WithTokenCallback(func(TokenUpdate))` | none; called for every Keepa envelope, including errors |
| `WithTokenReserve(int)` | 20 |
| `WithTimeout(time.Duration)` | 1 minute, applied only when your context has no deadline |

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
  can re-queue instead of blocking.
- The first paid call fetches `GetTokenStatus` (free) so the floor applies
  immediately. `client.Tokens()` returns the current projection.

## Working with history

Keepa timestamps are minutes since 2011. `keepa.Time` converts them with
`.Time()`. Price and rank series are `keepa.History` values with `Pairs()`
for `[time, value]` series and `Triples()` for the shipping-inclusive series
(`CSVType.HasShipping` tells you which). `CSVType` constants index the
statistics arrays: `stats.Current[keepa.CSVAmazon]`.

## Licence

MIT
````

- [ ] **Step 4: Run the full verification**

Run from `/Users/jameseagle/code/Jleagle/keepa`:

```sh
gofmt -l .
go vet ./...
go test -race -count=1 ./...
go mod tidy && test ! -f go.sum && echo "no dependencies"
git -C /Users/jameseagle/code/Jleagle/keepa status --short
```

Expected: gofmt prints nothing, vet is clean, tests report `ok`, `go.sum` does not exist, and the status shows only the files this plan created (plus `docs/`).

- [ ] **Step 5: Commit**

```bash
git -C /Users/jameseagle/code/Jleagle/keepa add README.md .github/workflows/test.yml example_test.go
git -C /Users/jameseagle/code/Jleagle/keepa commit -m "Add README, CI workflow and compiled examples"
```

---

## Self-review notes

- Every spec section maps to a task: 3 and 4 (Task 1, Task 15), 5 (Task 5), 6 (Task 1), 7 (Task 2), 8 (Task 3), 9 (Task 6), 10 (Tasks 5 and 7), 11 (Task 4), 12 (Task 5), 13.1 to 13.8 (Tasks 8 to 14 and 7), 14 (Task 8), 15 (every task), 16 (Task 15).
- An envelope whose `refillRate` is 0 is not recorded as a bucket reading, because Keepa returns all-zero token fields with parameter errors and recording them would make the next paid call wait on a fictional empty bucket. This was found while planning and is now in spec section 9, step 8 (Task 6, `TestDoIgnoresEnvelopeWithoutRefillRate`).
- All seven params structs and the endpoint option func types live in `options.go` (Task 5) so that `CallOption` compiles before any endpoint exists; endpoint files define only option constructors, methods and response types.
