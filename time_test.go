package keepa

import (
	"slices"
	"testing"
	"time"
)

func TestTimeConversion(t *testing.T) {
	// Keepa minute 0 is 2011-01-01T00:00:00Z.
	if got := Time(0).Time(); !got.Equal(time.Date(2011, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Time(0).Time() = %v", got)
	}
	// The consumer formula in price-spider: unix = (keepaMinutes + 21564000) * 60.
	want := time.Unix((1000+21564000)*60, 0).UTC()
	if got := Time(1000).Time(); !got.Equal(want) {
		t.Errorf("Time(1000).Time() = %v, want %v", got, want)
	}
	if got := Time(1000).Time().Location(); got != time.UTC {
		t.Errorf("Time().Location() = %v, want UTC", got)
	}
	if got := TimeOf(want); got != 1000 {
		t.Errorf("TimeOf(%v) = %d, want 1000", want, got)
	}
	if got := TimeOf(want.Add(59 * time.Second)); got != 1000 {
		t.Errorf("TimeOf did not truncate seconds: got %d, want 1000", got)
	}
}

func TestTimeValidAndString(t *testing.T) {
	for _, v := range []Time{0, -1} {
		if v.Valid() {
			t.Errorf("Time(%d).Valid() = true, want false", v)
		}
		if got := v.String(); got != "" {
			t.Errorf("Time(%d).String() = %q, want empty", v, got)
		}
	}
	if !Time(1).Valid() {
		t.Error("Time(1).Valid() = false, want true")
	}
	if got := Time(1000).String(); got != "2011-01-01T16:40:00Z" {
		t.Errorf("Time(1000).String() = %q, want 2011-01-01T16:40:00Z", got)
	}
}

func collectPairs(h History) (times []Time, values []int) {
	for tm, v := range h.Pairs() {
		times = append(times, tm)
		values = append(values, v)
	}
	return times, values
}

func TestHistoryPairs(t *testing.T) {
	// Trailing 30 has no value and must be ignored; -1 is passed through.
	times, values := collectPairs(History{10, 100, 20, -1, 30})
	if !slices.Equal(times, []Time{10, 20}) || !slices.Equal(values, []int{100, -1}) {
		t.Errorf("Pairs() = %v %v, want [10 20] [100 -1]", times, values)
	}
	if times, _ := collectPairs(nil); len(times) != 0 {
		t.Errorf("Pairs() on nil yielded %v", times)
	}
	if times, _ := collectPairs(History{5}); len(times) != 0 {
		t.Errorf("Pairs() on a single element yielded %v", times)
	}
}

func TestHistoryPairsStopsOnBreak(t *testing.T) {
	n := 0
	for range (History{1, 2, 3, 4, 5, 6}).Pairs() {
		n++
		break
	}
	if n != 1 {
		t.Errorf("iteration continued after break: %d", n)
	}
}

func TestHistoryTriples(t *testing.T) {
	// Trailing group [30, 300] is incomplete and must be ignored.
	var times []Time
	var points []PriceShipping
	for tm, p := range (History{10, 100, 5, 20, 200, 0, 30, 300}).Triples() {
		times = append(times, tm)
		points = append(points, p)
	}
	want := []PriceShipping{{Price: 100, Shipping: 5}, {Price: 200, Shipping: 0}}
	if !slices.Equal(times, []Time{10, 20}) || !slices.Equal(points, want) {
		t.Errorf("Triples() = %v %v, want [10 20] %v", times, points, want)
	}
	n := 0
	for range History(nil).Triples() {
		n++
	}
	if n != 0 {
		t.Errorf("Triples() on nil yielded %d groups", n)
	}
}
