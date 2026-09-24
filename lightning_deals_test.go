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
