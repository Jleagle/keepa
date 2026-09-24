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
	if len(s.CSV) != 2 || !slices.Equal(s.CSV[1], History{7200000, 9900, 7204320, 10000}) {
		t.Errorf("csv = %v", s.CSV)
	}
	if !slices.Equal(s.TotalStorefrontAsins, []int{7204320, 1200}) || !slices.Equal(s.ASINList, []string{"B000000001"}) || !slices.Equal(s.Address, []string{"1 Example Street", "US"}) {
		t.Errorf("storefront = %+v", s)
	}
}
