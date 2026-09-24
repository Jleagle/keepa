package keepa

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestCSVUnmarshalPositional(t *testing.T) {
	raw := `[[1,100,2,200],null,[3,300]]`
	var c CSV
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Amazon, History{1, 100, 2, 200}) {
		t.Errorf("Amazon = %v", c.Amazon)
	}
	if c.New != nil {
		t.Errorf("New = %v, want nil for a null entry", c.New)
	}
	if !slices.Equal(c.Used, History{3, 300}) {
		t.Errorf("Used = %v", c.Used)
	}
	if c.Sales != nil {
		t.Errorf("Sales = %v, want nil for an absent entry", c.Sales)
	}
	if got := c.Get(CSVUsed); !slices.Equal(got, History{3, 300}) {
		t.Errorf("Get(CSVUsed) = %v", got)
	}
	if got := c.Get(CSVType(99)); got != nil {
		t.Errorf("Get(99) = %v, want nil", got)
	}
	if got := c.Get(CSVType(-1)); got != nil {
		t.Errorf("Get(-1) = %v, want nil", got)
	}
}

func TestCSVUnmarshalAllIndices(t *testing.T) {
	series := make([][]int, int(csvTypeCount))
	for i := range series {
		series[i] = []int{i, i}
	}
	raw, err := json.Marshal(series)
	if err != nil {
		t.Fatal(err)
	}
	var c CSV
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	for i := CSVType(0); i < csvTypeCount; i++ {
		if got := c.Get(i); !slices.Equal(got, History{int(i), int(i)}) {
			t.Errorf("Get(%s) = %v, want [%d %d]", i, got, i, i)
		}
	}
	if !slices.Equal(c.Amazon, History{0, 0}) || !slices.Equal(c.CountNewFBM, History{35, 35}) {
		t.Errorf("first/last fields wrong: %v %v", c.Amazon, c.CountNewFBM)
	}
}

func TestCSVUnmarshalExtraSeriesIgnored(t *testing.T) {
	series := make([][]int, 40)
	raw, _ := json.Marshal(series)
	var c CSV
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("40 series should decode: %v", err)
	}
}

func TestCSVUnmarshalNull(t *testing.T) {
	c := CSV{Amazon: History{1, 2}}
	if err := json.Unmarshal([]byte(`null`), &c); err != nil {
		t.Fatal(err)
	}
	if c.Amazon != nil {
		t.Errorf("null should clear fields, Amazon = %v", c.Amazon)
	}
}

func TestCSVUnmarshalInvalid(t *testing.T) {
	var c CSV
	if err := json.Unmarshal([]byte(`{"a":1}`), &c); err == nil {
		t.Error("an object should not decode as CSV")
	}
}

func TestCSVTypeHasShipping(t *testing.T) {
	want := map[CSVType]bool{
		CSVNewFBMShipping: true, CSVBuyBoxShipping: true, CSVUsedNewShipping: true,
		CSVUsedVeryGoodShipping: true, CSVUsedGoodShipping: true, CSVUsedAcceptableShipping: true,
		CSVCollectibleNewShipping: true, CSVCollectibleVeryGoodShipping: true,
		CSVCollectibleGoodShipping: true, CSVCollectibleAcceptableShipping: true,
		CSVRefurbishedShipping: true, CSVEbayNewShipping: true, CSVEbayUsedShipping: true,
		CSVBuyBoxUsedShipping: true,
	}
	for i := CSVType(0); i < csvTypeCount; i++ {
		if got := i.HasShipping(); got != want[i] {
			t.Errorf("%s.HasShipping() = %v, want %v", i, got, want[i])
		}
	}
}

func TestCSVTypeStringAndValid(t *testing.T) {
	tests := map[CSVType]string{
		CSVAmazon:         "AMAZON",
		CSVNewFBMShipping: "NEW_FBM_SHIPPING",
		CSVPrimeExclusive: "PRIME_EXCL",
		CSVCountNewFBM:    "COUNT_NEW_FBM",
		CSVType(36):       "CSVType(36)",
		CSVType(-1):       "CSVType(-1)",
	}
	for typ, want := range tests {
		if got := typ.String(); got != want {
			t.Errorf("CSVType(%d).String() = %q, want %q", typ, got, want)
		}
	}
	if !CSVAmazon.Valid() || !CSVCountNewFBM.Valid() || CSVType(36).Valid() || CSVType(-1).Valid() {
		t.Error("Valid() boundaries wrong")
	}
	if int(csvTypeCount) != 36 {
		t.Errorf("csvTypeCount = %d, want 36", csvTypeCount)
	}
}

func TestCSVRoundTrip(t *testing.T) {
	var c CSV
	if err := json.Unmarshal([]byte(`[[1,100,2,200],null,[3,300]]`), &c); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var back CSV
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(back.Amazon, History{1, 100, 2, 200}) || back.New != nil || !slices.Equal(back.Used, History{3, 300}) {
		t.Errorf("round trip: amazon=%v new=%v used=%v (json %s)", back.Amazon, back.New, back.Used, b)
	}
	if pb, err := json.Marshal(&c); err != nil || string(pb) != string(b) {
		t.Errorf("*CSV marshals differently: %s, %v", pb, err)
	}

	empty, err := json.Marshal(CSV{})
	if err != nil {
		t.Fatal(err)
	}
	var nulls []any
	if err := json.Unmarshal(empty, &nulls); err != nil {
		t.Fatal(err)
	}
	if len(nulls) != 36 || slices.ContainsFunc(nulls, func(v any) bool { return v != nil }) {
		t.Errorf("empty CSV = %s, want 36 nulls", empty)
	}
}
