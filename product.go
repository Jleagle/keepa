package keepa

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// keepaStart is the earliest date Keepa holds data for. WithStats clamps to it.
var keepaStart = time.Date(2011, 1, 1, 0, 0, 0, 0, time.UTC)

// WithStats requests the statistics object computed over the interval from
// since until now. Dates before 2011 are clamped to 2011-01-01. No extra tokens.
func WithStats(since time.Time) ProductOption {
	return productOption(func(p *productParams) { p.stats = &since })
}

// WithRatings includes rating and review count history. One extra token per ASIN.
func WithRatings() ProductOption {
	return productOption(func(p *productParams) { p.ratings = true })
}

// WithLiveUpdate asks Keepa to refresh each product before answering
// (update=0). One extra token per ASIN, which Keepa waives when its data is
// already fresh.
func WithLiveUpdate() ProductOption {
	return productOption(func(p *productParams) { p.liveUpdate = true })
}

// WithBuyBox adds Buy Box data to the statistics object. Two extra tokens per ASIN.
func WithBuyBox() ProductOption {
	return productOption(func(p *productParams) { p.buyBox = true })
}

// WithVideos includes video metadata. No extra tokens.
func WithVideos() ProductOption {
	return productOption(func(p *productParams) { p.videos = true })
}

// WithoutHistory omits the csv history arrays from the response. No extra tokens.
func WithoutHistory() ProductOption {
	return productOption(func(p *productParams) { p.noHistory = true })
}

// GetProducts retrieves between 1 and 100 products by ASIN.
// Cost: 1 token per ASIN plus the extras documented on each option.
func (c *Client) GetProducts(ctx context.Context, domain Domain, asins []string, opts ...ProductOption) (*ProductResponse, error) {
	if !domain.Valid() {
		return nil, invalidRequest("unknown domain %d", domain)
	}
	if len(asins) == 0 || len(asins) > 100 {
		return nil, invalidRequest("between 1 and 100 ASINs required, got %d", len(asins))
	}
	var p productParams
	for _, o := range opts {
		o.applyProduct(&p)
	}

	q := c.query()
	q.Set("domain", domain.queryValue())
	q.Set("asin", strings.Join(asins, ","))
	perASIN := 1
	if p.stats != nil {
		since := *p.stats
		if since.Before(keepaStart) {
			since = keepaStart
		}
		q.Set("stats", strconv.FormatInt(since.UnixMilli(), 10)+","+strconv.FormatInt(c.now().UnixMilli(), 10))
	}
	if p.ratings {
		q.Set("rating", "1")
		perASIN++
	}
	if p.liveUpdate {
		q.Set("update", "0")
		perASIN++
	}
	if p.buyBox {
		q.Set("buybox", "1")
		perASIN += 2
	}
	if p.videos {
		q.Set("videos", "1")
	}
	if p.noHistory {
		q.Set("history", "0")
	}

	return do[ProductResponse](ctx, c, request{
		path:       "/product",
		query:      q,
		cost:       len(asins) * perASIN,
		callParams: p.callParams,
	})
}

// ProductResponse is returned by GetProducts.
type ProductResponse struct {
	Envelope
	Products []Product `json:"products"`
}
