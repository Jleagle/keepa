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
	Address                  []string  `json:"address"`            // business address lines, country code last
	ASINList                 []string  `json:"asinList"`           // storefront ASINs, when requested
	ASINListLastSeen         []Time    `json:"asinListLastSeen"`   // parallel to ASINList
	CSV                      []History `json:"csv"`                // 0: rating percentage history, 1: rating count history
	CurrentRating            int       `json:"currentRating"`      // percentage
	CurrentRatingCount       int       `json:"currentRatingCount"` // lifetime ratings
	DomainID                 int       `json:"domainId"`
	HasFBA                   bool      `json:"hasFBA"`
	IsScammer                bool      `json:"isScammer"`
	LastUpdate               Time      `json:"lastUpdate"`
	RatingsLast30Days        int       `json:"ratingsLast30Days"`
	SellerBrandStatistics    any       `json:"sellerBrandStatistics"`
	SellerCategoryStatistics any       `json:"sellerCategoryStatistics"`
	SellerID                 string    `json:"sellerId"`
	SellerName               string    `json:"sellerName"`
	ShipsFromChina           bool      `json:"shipsFromChina"`
	TotalStorefrontAsins     []int     `json:"totalStorefrontAsins"` // [Keepa minutes, count]
	TrackedSince             Time      `json:"trackedSince"`
}
