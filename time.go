package keepa

import (
	"iter"
	"time"
)

// keepaEpochMinutes is the number of minutes from the Unix epoch to
// 2011-01-01 00:00 UTC, the start of Keepa time.
const keepaEpochMinutes = 21564000

// Time is a Keepa timestamp: whole minutes since 2011-01-01 00:00 UTC.
// Keepa uses 0 and -1 where a timestamp is unknown; see Valid.
type Time int

// Time converts t to a time.Time in UTC.
func (t Time) Time() time.Time {
	return time.Unix((int64(t)+keepaEpochMinutes)*60, 0).UTC()
}

// Valid reports whether t holds a real timestamp.
func (t Time) Valid() bool { return t > 0 }

// String formats t as RFC 3339, or returns "" when t is not valid.
func (t Time) String() string {
	if !t.Valid() {
		return ""
	}
	return t.Time().Format(time.RFC3339)
}

// TimeOf converts a time.Time to Keepa minutes, truncating to the minute.
func TimeOf(t time.Time) Time {
	return Time(t.Unix()/60 - keepaEpochMinutes)
}

// History is a raw Keepa time series as sent in a product's csv field.
// Most series are [time, value, time, value, ...]; the shipping-inclusive
// price types are [time, price, shipping, ...]. See CSVType.HasShipping.
type History []int

// Pairs iterates a [time, value, ...] series. A trailing incomplete group is
// ignored. Values are not filtered; Keepa sends -1 for "no data".
func (h History) Pairs() iter.Seq2[Time, int] {
	return func(yield func(Time, int) bool) {
		for i := 0; i+1 < len(h); i += 2 {
			if !yield(Time(h[i]), h[i+1]) {
				return
			}
		}
	}
}

// PriceShipping is one point of a shipping-inclusive series.
type PriceShipping struct {
	Price    int
	Shipping int
}

// Triples iterates a [time, price, shipping, ...] series. A trailing
// incomplete group is ignored.
func (h History) Triples() iter.Seq2[Time, PriceShipping] {
	return func(yield func(Time, PriceShipping) bool) {
		for i := 0; i+2 < len(h); i += 3 {
			if !yield(Time(h[i]), PriceShipping{Price: h[i+1], Shipping: h[i+2]}) {
				return
			}
		}
	}
}
