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
