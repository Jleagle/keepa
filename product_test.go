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

	// A decoded product round-trips through encoding/json.
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var back Product
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(back.CSV.Amazon, p.CSV.Amazon) || back.Stats == nil || !slices.Equal(back.Stats.Current, p.Stats.Current) {
		t.Errorf("round trip: amazon=%v stats=%+v", back.CSV.Amazon, back.Stats)
	}
}

func TestProductLastCategoryEmpty(t *testing.T) {
	if got := (Product{}).LastCategory(); got != (CategoryNode{}) {
		t.Errorf("LastCategory() on empty tree = %+v", got)
	}
}
