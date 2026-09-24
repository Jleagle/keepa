package keepa

import "testing"

func TestCallOptionsApplyToEveryEndpoint(t *testing.T) {
	endpoints := []struct {
		name  string
		apply func(CallOption) callParams
	}{
		{"product", func(o CallOption) callParams { var p productParams; o.applyProduct(&p); return p.callParams }},
		{"bestsellers", func(o CallOption) callParams { var p bestSellersParams; o.applyBestSellers(&p); return p.callParams }},
		{"category", func(o CallOption) callParams { var p categoryParams; o.applyCategory(&p); return p.callParams }},
		{"deals", func(o CallOption) callParams { var p dealsParams; o.applyDeals(&p); return p.callParams }},
		{"lightning", func(o CallOption) callParams {
			var p lightningDealsParams
			o.applyLightningDeals(&p)
			return p.callParams
		}},
		{"search", func(o CallOption) callParams { var p searchParams; o.applySearch(&p); return p.callParams }},
		{"sellers", func(o CallOption) callParams { var p sellersParams; o.applySellers(&p); return p.callParams }},
	}
	for _, e := range endpoints {
		got := e.apply(WithReserve(5))
		if got.reserve == nil || *got.reserve != 5 {
			t.Errorf("%s: WithReserve(5) not applied: %+v", e.name, got)
		}
		if got.noWait {
			t.Errorf("%s: WithReserve must not set noWait", e.name)
		}
		got = e.apply(WithoutWaiting())
		if !got.noWait {
			t.Errorf("%s: WithoutWaiting not applied", e.name)
		}
		if got.reserve != nil {
			t.Errorf("%s: WithoutWaiting must not set reserve", e.name)
		}
	}
}
