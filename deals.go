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
