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
