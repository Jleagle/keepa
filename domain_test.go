package keepa

import (
	"strconv"
	"testing"
)

func TestDomainTLD(t *testing.T) {
	tests := []struct {
		domain Domain
		tld    string
		str    string
	}{
		{DomainUS, "com", "amazon.com"},
		{DomainGB, "co.uk", "amazon.co.uk"},
		{DomainDE, "de", "amazon.de"},
		{DomainFR, "fr", "amazon.fr"},
		{DomainJP, "co.jp", "amazon.co.jp"},
		{DomainCA, "ca", "amazon.ca"},
		{DomainIT, "it", "amazon.it"},
		{DomainES, "es", "amazon.es"},
		{DomainIN, "in", "amazon.in"},
		{DomainMX, "com.mx", "amazon.com.mx"},
		{DomainBR, "com.br", "amazon.com.br"},
	}
	for _, tt := range tests {
		if got := tt.domain.TLD(); got != tt.tld {
			t.Errorf("Domain(%d).TLD() = %q, want %q", tt.domain, got, tt.tld)
		}
		if got := tt.domain.String(); got != tt.str {
			t.Errorf("Domain(%d).String() = %q, want %q", tt.domain, got, tt.str)
		}
		if !tt.domain.Valid() {
			t.Errorf("Domain(%d).Valid() = false, want true", tt.domain)
		}
		if got := tt.domain.queryValue(); got != strconv.Itoa(int(tt.domain)) {
			t.Errorf("Domain(%d).queryValue() = %q", tt.domain, got)
		}
	}
}

func TestDomainInvalid(t *testing.T) {
	for _, d := range []Domain{0, 7, 13, -1} {
		if d.Valid() {
			t.Errorf("Domain(%d).Valid() = true, want false", d)
		}
		if got := d.TLD(); got != "" {
			t.Errorf("Domain(%d).TLD() = %q, want empty", d, got)
		}
	}
	if got := Domain(7).String(); got != "Domain(7)" {
		t.Errorf("Domain(7).String() = %q, want Domain(7)", got)
	}
}

func TestDomainFromTLD(t *testing.T) {
	d, ok := DomainFromTLD("co.uk")
	if !ok || d != DomainGB {
		t.Errorf("DomainFromTLD(co.uk) = %d, %v; want DomainGB, true", d, ok)
	}
	if _, ok := DomainFromTLD("cn"); ok {
		t.Error("DomainFromTLD(cn) found a domain; Keepa does not serve it")
	}
	if _, ok := DomainFromTLD(""); ok {
		t.Error("DomainFromTLD(\"\") found a domain")
	}
}
