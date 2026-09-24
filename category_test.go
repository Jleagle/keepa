package keepa

import (
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"testing"
)

func TestGetCategoriesRequestDefaults(t *testing.T) {
	c, rec := newTestClient(t, serveFixture(t, "categories.json"))
	if _, err := c.GetCategories(t.Context(), DomainDE, []int64{172282, 502394}); err != nil {
		t.Fatal(err)
	}
	last := rec.Last(t)
	if last.method != http.MethodGet || last.path != "/category" {
		t.Errorf("request = %s %s", last.method, last.path)
	}
	// Keepa lists parents as required, so 0 is sent explicitly.
	want := url.Values{"key": {"test-key"}, "domain": {"3"}, "category": {"172282,502394"}, "parents": {"0"}}
	if !maps.EqualFunc(last.query, want, slices.Equal) {
		t.Errorf("query = %v, want exactly %v", last.query, want)
	}
}

func TestGetCategoriesWithParents(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope(`"categories":{}`)))
	if _, err := c.GetCategories(t.Context(), DomainUS, []int64{1}, WithParents()); err != nil {
		t.Fatal(err)
	}
	if got := rec.Last(t).query.Get("parents"); got != "1" {
		t.Errorf("parents = %q, want 1", got)
	}
}

func TestGetCategoriesValidation(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	tests := []struct {
		name   string
		domain Domain
		ids    []int64
	}{
		{"no ids", DomainUS, nil},
		{"eleven ids", DomainUS, make([]int64, 11)},
		{"reserved domain", Domain(7), []int64{1}},
	}
	for _, tt := range tests {
		if _, err := c.GetCategories(t.Context(), tt.domain, tt.ids); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: err = %v, want ErrInvalidRequest", tt.name, err)
		}
	}
	if n := len(rec.Calls()); n != 0 {
		t.Errorf("%d requests sent before validation", n)
	}
	if _, err := c.GetCategories(t.Context(), DomainUS, make([]int64, 10)); err != nil {
		t.Errorf("10 ids must be allowed: %v", err)
	}
}

func TestGetCategoriesCost(t *testing.T) {
	c, _ := newTestClient(t, serveJSON(200, okEnvelope("")))
	_, err := c.GetCategories(t.Context(), DomainUS, []int64{1, 2, 3}, WithReserve(1200), WithoutWaiting())
	if got := costOf(t, err); got != 1 {
		t.Errorf("cost = %d, want 1 regardless of the number of ids", got)
	}
}

func TestGetCategoriesDecodes(t *testing.T) {
	c, _ := newTestClient(t, serveFixture(t, "categories.json"))
	res, err := c.GetCategories(t.Context(), DomainUS, []int64{172282, 502394})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Categories) != 2 {
		t.Fatalf("categories = %d, want 2", len(res.Categories))
	}
	root := res.Categories[172282]
	if root.Name != "Electronics" || root.Parent != 0 || !slices.Equal(root.Children, []int64{502394}) || root.ProductCount != 123456 || !root.IsBrowseNode {
		t.Errorf("root = %+v", root)
	}
	if root.AvgRating != 42 || root.IsFBAPercent != 55.5 || !slices.Equal(root.TopBrands, []string{"Acme", "Globex"}) {
		t.Errorf("root stats = %+v", root)
	}
	child := res.Categories[502394]
	if child.Name != "Camera & Photo" || child.Parent != 172282 || len(child.Children) != 0 {
		t.Errorf("child = %+v", child)
	}
	if len(res.CategoryParents) != 0 {
		t.Errorf("categoryParents = %v, want empty", res.CategoryParents)
	}
}
