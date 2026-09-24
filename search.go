package keepa

import (
	"context"
	"strings"
)

// WithASINsOnly returns only the ASINList instead of full product objects.
func WithASINsOnly() SearchOption {
	return searchOption(func(p *searchParams) { p.asinsOnly = true })
}

// SearchProducts searches a marketplace by keyword and returns up to 20
// products. Cost: 10 tokens.
func (c *Client) SearchProducts(ctx context.Context, domain Domain, term string, opts ...SearchOption) (*SearchResponse, error) {
	if !domain.Valid() {
		return nil, invalidRequest("unknown domain %d", domain)
	}
	if strings.TrimSpace(term) == "" {
		return nil, invalidRequest("search term required")
	}
	var p searchParams
	for _, o := range opts {
		o.applySearch(&p)
	}

	q := c.query()
	q.Set("domain", domain.queryValue())
	q.Set("type", "product")
	q.Set("term", term)
	if p.asinsOnly {
		q.Set("asins-only", "1")
	}

	return do[SearchResponse](ctx, c, request{
		path:       "/search",
		query:      q,
		cost:       10,
		callParams: p.callParams,
	})
}

// SearchResponse is returned by SearchProducts. Products is empty when
// WithASINsOnly was used.
type SearchResponse struct {
	Envelope
	Products []Product `json:"products"`
	ASINList []string  `json:"asinList"`
}
