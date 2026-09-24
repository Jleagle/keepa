package keepa

import (
	"fmt"
	"strconv"
)

// Domain identifies an Amazon marketplace by Keepa's numeric domain ID.
type Domain int

const (
	DomainUS Domain = 1  // amazon.com
	DomainGB Domain = 2  // amazon.co.uk
	DomainDE Domain = 3  // amazon.de
	DomainFR Domain = 4  // amazon.fr
	DomainJP Domain = 5  // amazon.co.jp
	DomainCA Domain = 6  // amazon.ca
	DomainIT Domain = 8  // amazon.it
	DomainES Domain = 9  // amazon.es
	DomainIN Domain = 10 // amazon.in
	DomainMX Domain = 11 // amazon.com.mx
	DomainBR Domain = 12 // amazon.com.br
)

// ID 7 is reserved by Keepa and deliberately absent.
var domainTLDs = map[Domain]string{
	DomainUS: "com",
	DomainGB: "co.uk",
	DomainDE: "de",
	DomainFR: "fr",
	DomainJP: "co.jp",
	DomainCA: "ca",
	DomainIT: "it",
	DomainES: "es",
	DomainIN: "in",
	DomainMX: "com.mx",
	DomainBR: "com.br",
}

// TLD returns the Amazon top-level domain, such as "co.uk", or "" when d is not valid.
func (d Domain) TLD() string { return domainTLDs[d] }

// Valid reports whether d is a marketplace Keepa serves.
func (d Domain) Valid() bool {
	_, ok := domainTLDs[d]
	return ok
}

// String returns the Amazon host name, such as "amazon.co.uk".
func (d Domain) String() string {
	if tld, ok := domainTLDs[d]; ok {
		return "amazon." + tld
	}
	return fmt.Sprintf("Domain(%d)", int(d))
}

// DomainFromTLD returns the Domain for an Amazon top-level domain such as "co.uk".
func DomainFromTLD(tld string) (Domain, bool) {
	for d, t := range domainTLDs {
		if t == tld {
			return d, true
		}
	}
	return 0, false
}

func (d Domain) queryValue() string { return strconv.Itoa(int(d)) }
