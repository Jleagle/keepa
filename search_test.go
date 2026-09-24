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
