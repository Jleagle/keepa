package keepa

import (
	"encoding/json"
	"fmt"
)

// CSVType indexes the price-type series in a product's csv field and in the
// per-price-type arrays of ProductStats and Deal.
type CSVType int

const (
	CSVAmazon CSVType = iota
	CSVNew
	CSVUsed
	CSVSales
	CSVListPrice
	CSVCollectible
	CSVRefurbished
	CSVNewFBMShipping
	CSVLightningDeal
	CSVWarehouse
	CSVNewFBA
	CSVCountNew
	CSVCountUsed
	CSVCountRefurbished
	CSVCountCollectible
	CSVExtraInfoUpdates
	CSVRating
	CSVCountReviews
	CSVBuyBoxShipping
	CSVUsedNewShipping
	CSVUsedVeryGoodShipping
	CSVUsedGoodShipping
	CSVUsedAcceptableShipping
	CSVCollectibleNewShipping
	CSVCollectibleVeryGoodShipping
	CSVCollectibleGoodShipping
	CSVCollectibleAcceptableShipping
	CSVRefurbishedShipping
	CSVEbayNewShipping
	CSVEbayUsedShipping
	CSVTradeIn
	CSVRental
	CSVBuyBoxUsedShipping
	CSVPrimeExclusive
	CSVCountNewFBA
	CSVCountNewFBM

	csvTypeCount // number of known series
)

var csvTypeNames = [csvTypeCount]string{
	"AMAZON", "NEW", "USED", "SALES", "LISTPRICE", "COLLECTIBLE", "REFURBISHED",
	"NEW_FBM_SHIPPING", "LIGHTNING_DEAL", "WAREHOUSE", "NEW_FBA", "COUNT_NEW",
	"COUNT_USED", "COUNT_REFURBISHED", "COUNT_COLLECTIBLE", "EXTRA_INFO_UPDATES",
	"RATING", "COUNT_REVIEWS", "BUY_BOX_SHIPPING", "USED_NEW_SHIPPING",
	"USED_VERY_GOOD_SHIPPING", "USED_GOOD_SHIPPING", "USED_ACCEPTABLE_SHIPPING",
	"COLLECTIBLE_NEW_SHIPPING", "COLLECTIBLE_VERY_GOOD_SHIPPING",
	"COLLECTIBLE_GOOD_SHIPPING", "COLLECTIBLE_ACCEPTABLE_SHIPPING",
	"REFURBISHED_SHIPPING", "EBAY_NEW_SHIPPING", "EBAY_USED_SHIPPING", "TRADE_IN",
	"RENT", "BUY_BOX_USED_SHIPPING", "PRIME_EXCL", "COUNT_NEW_FBA", "COUNT_NEW_FBM",
}

// Valid reports whether t is a series Keepa documents.
func (t CSVType) Valid() bool { return t >= 0 && t < csvTypeCount }

// String returns Keepa's name for the series, such as "BUY_BOX_SHIPPING".
func (t CSVType) String() string {
	if t.Valid() {
		return csvTypeNames[t]
	}
	return fmt.Sprintf("CSVType(%d)", int(t))
}

// HasShipping reports whether the series holds [time, price, shipping]
// triples rather than [time, value] pairs. Keepa documents this for types
// 7, 18 through 29 and 32.
func (t CSVType) HasShipping() bool {
	switch {
	case t == CSVNewFBMShipping, t == CSVBuyBoxUsedShipping:
		return true
	case t >= CSVBuyBoxShipping && t <= CSVEbayUsedShipping:
		return true
	}
	return false
}

// CSV holds a product's price and rank history, one series per CSVType.
// Keepa sends it as a positional array of arrays; fields are filled by index.
type CSV struct {
	Amazon                        History
	New                           History
	Used                          History
	Sales                         History
	ListPrice                     History
	Collectible                   History
	Refurbished                   History
	NewFBMShipping                History
	LightningDeal                 History
	Warehouse                     History
	NewFBA                        History
	CountNew                      History
	CountUsed                     History
	CountRefurbished              History
	CountCollectible              History
	ExtraInfoUpdates              History
	Rating                        History
	CountReviews                  History
	BuyBoxShipping                History
	UsedNewShipping               History
	UsedVeryGoodShipping          History
	UsedGoodShipping              History
	UsedAcceptableShipping        History
	CollectibleNewShipping        History
	CollectibleVeryGoodShipping   History
	CollectibleGoodShipping       History
	CollectibleAcceptableShipping History
	RefurbishedShipping           History
	EbayNewShipping               History
	EbayUsedShipping              History
	TradeIn                       History
	Rental                        History
	BuyBoxUsedShipping            History
	PrimeExclusive                History
	CountNewFBA                   History
	CountNewFBM                   History
}

func (c *CSV) fields() [csvTypeCount]*History {
	return [csvTypeCount]*History{
		&c.Amazon, &c.New, &c.Used, &c.Sales, &c.ListPrice, &c.Collectible,
		&c.Refurbished, &c.NewFBMShipping, &c.LightningDeal, &c.Warehouse,
		&c.NewFBA, &c.CountNew, &c.CountUsed, &c.CountRefurbished,
		&c.CountCollectible, &c.ExtraInfoUpdates, &c.Rating, &c.CountReviews,
		&c.BuyBoxShipping, &c.UsedNewShipping, &c.UsedVeryGoodShipping,
		&c.UsedGoodShipping, &c.UsedAcceptableShipping, &c.CollectibleNewShipping,
		&c.CollectibleVeryGoodShipping, &c.CollectibleGoodShipping,
		&c.CollectibleAcceptableShipping, &c.RefurbishedShipping,
		&c.EbayNewShipping, &c.EbayUsedShipping, &c.TradeIn, &c.Rental,
		&c.BuyBoxUsedShipping, &c.PrimeExclusive, &c.CountNewFBA, &c.CountNewFBM,
	}
}

// UnmarshalJSON decodes Keepa's positional array of series. A null entry
// leaves its field nil; series beyond the known range are ignored.
func (c *CSV) UnmarshalJSON(b []byte) error {
	var data []History
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	fields := c.fields()
	for i := range fields {
		if i < len(data) {
			*fields[i] = data[i]
		} else {
			*fields[i] = nil
		}
	}
	return nil
}

// Get returns the series for t, or nil when t is not valid.
func (c *CSV) Get(t CSVType) History {
	if !t.Valid() {
		return nil
	}
	return *c.fields()[t]
}
