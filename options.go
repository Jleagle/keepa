package keepa

import "time"

// callParams is the per-call policy every endpoint accepts.
type callParams struct {
	reserve *int // overrides Client.reserve when set
	noWait  bool // return *TokenWaitError instead of sleeping
}

// Per-endpoint parameter sets. The endpoint files define the option
// constructors that populate them.

type productParams struct {
	callParams
	stats      *time.Time
	ratings    bool
	liveUpdate bool
	buyBox     bool
	videos     bool
	noHistory  bool
}

type bestSellersParams struct {
	callParams
	rankRange  int
	variations bool
}

type categoryParams struct {
	callParams
	parents bool
}

type dealsParams struct {
	callParams
	page         int
	dateRange    *int
	minRating    *int
	outOfStock   bool
	filterErotic bool
	sort         *int
	deltaLast    *[2]int
}

type lightningDealsParams struct {
	callParams
}

type searchParams struct {
	callParams
	asinsOnly bool
}

type sellersParams struct {
	callParams
}

// ProductOption configures GetProducts.
type ProductOption interface{ applyProduct(*productParams) }

// BestSellersOption configures GetBestSellers.
type BestSellersOption interface{ applyBestSellers(*bestSellersParams) }

// CategoryOption configures GetCategories.
type CategoryOption interface{ applyCategory(*categoryParams) }

// DealsOption configures GetDeals.
type DealsOption interface{ applyDeals(*dealsParams) }

// LightningDealsOption configures GetLightningDeals.
type LightningDealsOption interface{ applyLightningDeals(*lightningDealsParams) }

// SearchOption configures SearchProducts.
type SearchOption interface{ applySearch(*searchParams) }

// SellersOption configures GetSellers.
type SellersOption interface{ applySellers(*sellersParams) }

type productOption func(*productParams)
type bestSellersOption func(*bestSellersParams)
type categoryOption func(*categoryParams)
type dealsOption func(*dealsParams)
type searchOption func(*searchParams)

func (o productOption) applyProduct(p *productParams)             { o(p) }
func (o bestSellersOption) applyBestSellers(p *bestSellersParams) { o(p) }
func (o categoryOption) applyCategory(p *categoryParams)          { o(p) }
func (o dealsOption) applyDeals(p *dealsParams)                   { o(p) }
func (o searchOption) applySearch(p *searchParams)                { o(p) }

// CallOption is accepted by every endpoint method.
type CallOption interface {
	ProductOption
	BestSellersOption
	CategoryOption
	DealsOption
	LightningDealsOption
	SearchOption
	SellersOption
}

type callOption func(*callParams)

func (o callOption) applyProduct(p *productParams)               { o(&p.callParams) }
func (o callOption) applyBestSellers(p *bestSellersParams)       { o(&p.callParams) }
func (o callOption) applyCategory(p *categoryParams)             { o(&p.callParams) }
func (o callOption) applyDeals(p *dealsParams)                   { o(&p.callParams) }
func (o callOption) applyLightningDeals(p *lightningDealsParams) { o(&p.callParams) }
func (o callOption) applySearch(p *searchParams)                 { o(&p.callParams) }
func (o callOption) applySellers(p *sellersParams)               { o(&p.callParams) }

// WithReserve overrides the client's token reserve for one call. Background
// work keeps the client floor; an interactive call can pass WithReserve(0)
// to run as long as the bucket can pay for it.
func WithReserve(n int) CallOption {
	return callOption(func(p *callParams) { p.reserve = &n })
}

// WithoutWaiting makes the call return a *TokenWaitError, matching
// ErrWouldWait, instead of sleeping when the bucket cannot pay for it while
// keeping the reserve. No slot is reserved.
func WithoutWaiting() CallOption {
	return callOption(func(p *callParams) { p.noWait = true })
}
