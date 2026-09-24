package keepa

import "context"

// GetLightningDeals retrieves every lightning deal of the last four days,
// active and expired. Cost: 500 tokens.
func (c *Client) GetLightningDeals(ctx context.Context, domain Domain, opts ...LightningDealsOption) (*LightningDealsResponse, error) {
	if !domain.Valid() {
		return nil, invalidRequest("unknown domain %d", domain)
	}
	var p lightningDealsParams
	for _, o := range opts {
		o.applyLightningDeals(&p)
	}

	q := c.query()
	q.Set("domain", domain.queryValue())

	return do[LightningDealsResponse](ctx, c, request{
		path:       "/lightningdeal",
		query:      q,
		cost:       500,
		callParams: p.callParams,
	})
}

// LightningDealsResponse is returned by GetLightningDeals.
type LightningDealsResponse struct {
	Envelope
	LightningDeals []LightningDeal `json:"lightningDeals"`
}

// LightningDeal is a Keepa lightning deal object. Prices are in the smallest
// currency unit; DealPrice is -1 while the deal is upcoming.
type LightningDeal struct {
	DomainID            int                  `json:"domainId"`
	LastUpdate          Time                 `json:"lastUpdate"`
	ASIN                string               `json:"asin"`
	Title               string               `json:"title"`
	SellerID            string               `json:"sellerId"`
	SellerName          string               `json:"sellerName"`
	DealID              string               `json:"dealId"`
	DealPrice           int                  `json:"dealPrice"`
	CurrentPrice        int                  `json:"currentPrice"`
	Image               string               `json:"image"`
	IsPrimeEligible     bool                 `json:"isPrimeEligible"`
	IsFulfilledByAmazon bool                 `json:"isFulfilledByAmazon"`
	Rating              int                  `json:"rating"` // 0 to 50
	TotalReviews        int                  `json:"totalReviews"`
	DealState           string               `json:"dealState"` // AVAILABLE, WAITLIST, SOLDOUT, WAITLISTFULL, EXPIRED, SUPPRESSED
	StartTime           Time                 `json:"startTime"`
	EndTime             Time                 `json:"endTime"`
	PercentClaimed      int                  `json:"percentClaimed"`
	PercentOff          int                  `json:"percentOff"`
	Variation           []VariationAttribute `json:"variation"`
}
