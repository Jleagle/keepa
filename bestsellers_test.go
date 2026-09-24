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
	// With a 100ms client timeout, a 130ms response succeeds only because
	// best sellers doubles the fallback deadline to 200ms.
	slow := func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(130 * time.Millisecond):
		}
		serveJSON(200, okEnvelope(`"asinList":[]`))(w, r)
	}
	c, _ := newTestClient(t, slow, WithTimeout(100*time.Millisecond))
	c.tokens.state = TokenState{Known: true, Left: 1200, RefillRate: 20, UpdatedAt: testNow}
	if _, err := c.GetBestSellers(t.Context(), DomainUS, 1); err != nil {
		t.Errorf("expected the doubled timeout to cover a 130ms response: %v", err)
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
