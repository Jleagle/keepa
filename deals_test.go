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
