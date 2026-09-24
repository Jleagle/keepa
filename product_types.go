package keepa

// Product is a Keepa product object. Prices are integers in the smallest
// unit of the marketplace currency; -1 means no data. Timestamps Keepa
// documents as Keepa minutes are typed Time. PublicationDate and ReleaseDate
// are YYYYMMDD integers, -1 when unknown.
type Product struct {
	ASIN                            string              `json:"asin"`
	Author                          *string             `json:"author"`
	AvailabilityAmazon              int                 `json:"availabilityAmazon"`
	AvailabilityAmazonDelay         []int               `json:"availabilityAmazonDelay"`
	BatteriesIncluded               bool                `json:"batteriesIncluded"`
	BatteriesRequired               bool                `json:"batteriesRequired"`
	Binding                         string              `json:"binding"`
	Brand                           string              `json:"brand"`
	BrandStoreName                  string              `json:"brandStoreName"`
	BrandStoreURL                   string              `json:"brandStoreUrl"`
	BrandStoreURLName               string              `json:"brandStoreUrlName"`
	BuyBoxEligibleOfferCounts       []int               `json:"buyBoxEligibleOfferCounts"`
	BuyBoxSellerIDHistory           any                 `json:"buyBoxSellerIdHistory"`
	Categories                      []int64             `json:"categories"`
	CategoryTree                    []CategoryNode      `json:"categoryTree"`
	Color                           string              `json:"color"`
	CompetitivePriceThreshold       int                 `json:"competitivePriceThreshold"`
	Contributors                    [][]string          `json:"contributors,omitempty"`
	Coupon                          any                 `json:"coupon"`
	CSV                             CSV                 `json:"csv"`
	Description                     string              `json:"description"`
	DomainID                        int                 `json:"domainId"`
	EANList                         []string            `json:"eanList"`
	EbayListingIDs                  []int64             `json:"ebayListingIds"`
	Edition                         string              `json:"edition"`
	FBAFees                         *FBAFees            `json:"fbaFees"`
	Features                        []string            `json:"features"`
	Format                          any                 `json:"format"`
	FrequentlyBoughtTogether        any                 `json:"frequentlyBoughtTogether"`
	G                               int                 `json:"g"`
	GTINList                        []string            `json:"gtinList"`
	HasReviews                      bool                `json:"hasReviews"`
	HazardousMaterials              []HazardousMaterial `json:"hazardousMaterials"`
	Images                          []Image             `json:"images"`
	IncludedComponents              string              `json:"includedComponents"`
	IsAdultProduct                  bool                `json:"isAdultProduct"`
	IsEligibleForSuperSaverShipping bool                `json:"isEligibleForSuperSaverShipping"`
	IsEligibleForTradeIn            bool                `json:"isEligibleForTradeIn"`
	IsHeatSensitive                 bool                `json:"isHeatSensitive"`
	IsRedirectASIN                  bool                `json:"isRedirectASIN"`
	IsSNS                           bool                `json:"isSNS"`
	ItemHeight                      int                 `json:"itemHeight"`
	ItemLength                      int                 `json:"itemLength"`
	ItemWeight                      int                 `json:"itemWeight"`
	ItemWidth                       int                 `json:"itemWidth"`
	Languages                       [][]string          `json:"languages"`
	LastEbayUpdate                  Time                `json:"lastEbayUpdate"`
	LastPriceChange                 Time                `json:"lastPriceChange"`
	LastRatingUpdate                Time                `json:"lastRatingUpdate"`
	LastSoldUpdate                  Time                `json:"lastSoldUpdate"`
	LastUpdate                      Time                `json:"lastUpdate"`
	Launchpad                       bool                `json:"launchpad"`
	ListedSince                     Time                `json:"listedSince"`
	LiveOffersOrder                 any                 `json:"liveOffersOrder"`
	Manufacturer                    string              `json:"manufacturer"`
	Model                           string              `json:"model"`
	MonthlySold                     int                 `json:"monthlySold"`
	MonthlySoldHistory              []int               `json:"monthlySoldHistory"`
	NumberOfItems                   int                 `json:"numberOfItems"`
	NumberOfPages                   int                 `json:"numberOfPages"`
	OffersSuccessful                bool                `json:"offersSuccessful"`
	PackageHeight                   int                 `json:"packageHeight"`
	PackageLength                   int                 `json:"packageLength"`
	PackageQuantity                 int                 `json:"packageQuantity"`
	PackageWeight                   int                 `json:"packageWeight"`
	PackageWidth                    int                 `json:"packageWidth"`
	ParentASIN                      string              `json:"parentAsin"`
	ParentASINHistory               []string            `json:"parentAsinHistory"`
	ParentTitle                     string              `json:"parentTitle"`
	PartNumber                      string              `json:"partNumber"`
	PrimeDealEndTime                int                 `json:"primeDealEndTime"`
	ProductGroup                    string              `json:"productGroup"`
	ProductType                     int                 `json:"productType"`
	Promotions                      any                 `json:"promotions"`
	PublicationDate                 int                 `json:"publicationDate"`
	ReferralFeePercent              int                 `json:"referralFeePercent"`
	ReferralFeePercentage           float64             `json:"referralFeePercentage"`
	ReleaseDate                     int                 `json:"releaseDate"`
	Reviews                         ProductReviews      `json:"reviews"`
	RootCategory                    int64               `json:"rootCategory"`
	SalesRankDisplayGroup           string              `json:"salesRankDisplayGroup"`
	SalesRankReference              int                 `json:"salesRankReference"`
	SalesRankReferenceHistory       []int               `json:"salesRankReferenceHistory"`
	SalesRanks                      map[int64][]int64   `json:"salesRanks"`
	Size                            string              `json:"size"`
	SpecificUsesForProduct          []string            `json:"specificUsesForProduct"`
	Stats                           *ProductStats       `json:"stats"`
	Style                           string              `json:"style"`
	Title                           string              `json:"title"`
	TrackingSince                   Time                `json:"trackingSince"`
	Type                            string              `json:"type"`
	UnitCount                       UnitCount           `json:"unitCount"`
	UPCList                         []string            `json:"upcList"`
	URLSlug                         string              `json:"urlSlug"`
	VariableClosingFee              int                 `json:"variableClosingFee"`
	Variations                      []Variation         `json:"variations"`
	Videos                          []Video             `json:"videos"`
	WebsiteDisplayGroup             string              `json:"websiteDisplayGroup"`
	WebsiteDisplayGroupName         string              `json:"websiteDisplayGroupName"`
}

// LastCategory returns the deepest node of the category tree, or the zero
// value when the tree is empty.
func (p Product) LastCategory() CategoryNode {
	if len(p.CategoryTree) == 0 {
		return CategoryNode{}
	}
	return p.CategoryTree[len(p.CategoryTree)-1]
}

// CategoryNode is one level of a product's category tree.
type CategoryNode struct {
	CatID int64  `json:"catId"`
	Name  string `json:"name"`
}

// Image is one product image in large and medium sizes.
type Image struct {
	L  string `json:"l"`
	LH int    `json:"lH"`
	LW int    `json:"lW"`
	M  string `json:"m"`
	MH int    `json:"mH"`
	MW int    `json:"mW"`
}

// Variation is a sibling product that differs in the listed attributes.
type Variation struct {
	ASIN       string               `json:"asin"`
	Image      string               `json:"image"`
	Attributes []VariationAttribute `json:"attributes"`
}

// VariationAttribute is one dimension of a variation, such as Color: Black.
type VariationAttribute struct {
	Dimension string `json:"dimension"`
	Value     string `json:"value"`
}

// Video is a product video.
type Video struct {
	Title    string `json:"title"`
	Image    string `json:"image"`
	Duration int    `json:"duration"`
	Creator  string `json:"creator"`
	Name     string `json:"name"`
	URL      string `json:"url"`
}

// FBAFees holds Keepa's last known fulfilment fee.
type FBAFees struct {
	LastUpdate     int `json:"lastUpdate"`
	PickAndPackFee int `json:"pickAndPackFee"`
}

// HazardousMaterial is one hazardous material aspect of a product.
type HazardousMaterial struct {
	Aspect string `json:"aspect"`
	Value  string `json:"value"`
}

// ProductReviews holds review and rating count history.
type ProductReviews struct {
	LastUpdate  Time    `json:"lastUpdate"`
	ReviewCount History `json:"reviewCount"`
	RatingCount History `json:"ratingCount"`
}

// UnitCount is the unit a product is sold by.
type UnitCount struct {
	UnitType  string  `json:"unitType"`
	UnitValue float64 `json:"unitValue"`
}

// ProductStats is the statistics object returned when WithStats is used.
// Arrays are indexed by CSVType; min and max hold [time, value] pairs.
type ProductStats struct {
	AtIntervalStart                []int   `json:"atIntervalStart"`
	Avg                            []int   `json:"avg"`
	Avg180                         []int   `json:"avg180"`
	Avg30                          []int   `json:"avg30"`
	Avg365                         []int   `json:"avg365"`
	Avg90                          []int   `json:"avg90"`
	BuyBoxAvailabilityMessage      *string `json:"buyBoxAvailabilityMessage"`
	BuyBoxCondition                any     `json:"buyBoxCondition"`
	BuyBoxIsAmazon                 *bool   `json:"buyBoxIsAmazon"`
	BuyBoxIsBackorder              any     `json:"buyBoxIsBackorder"`
	BuyBoxIsFBA                    any     `json:"buyBoxIsFBA"`
	BuyBoxIsFreeShippingEligible   any     `json:"buyBoxIsFreeShippingEligible"`
	BuyBoxIsMAP                    any     `json:"buyBoxIsMAP"`
	BuyBoxIsPreorder               any     `json:"buyBoxIsPreorder"`
	BuyBoxIsPrimeEligible          *bool   `json:"buyBoxIsPrimeEligible"`
	BuyBoxIsPrimeExclusive         any     `json:"buyBoxIsPrimeExclusive"`
	BuyBoxIsShippable              any     `json:"buyBoxIsShippable"`
	BuyBoxIsUnqualified            any     `json:"buyBoxIsUnqualified"`
	BuyBoxIsUsed                   any     `json:"buyBoxIsUsed"`
	BuyBoxIsWarehouseDeal          any     `json:"buyBoxIsWarehouseDeal"`
	BuyBoxPrice                    int     `json:"buyBoxPrice"`
	BuyBoxSellerID                 *string `json:"buyBoxSellerId"`
	BuyBoxShipping                 int     `json:"buyBoxShipping"`
	BuyBoxShippingCountry          any     `json:"buyBoxShippingCountry"`
	Current                        []int   `json:"current"`
	DeltaPercent90MonthlySold      any     `json:"deltaPercent90_monthlySold"`
	IsLowest                       []bool  `json:"isLowest"`
	IsLowest90                     []bool  `json:"isLowest90"`
	LastBuyBoxUpdate               any     `json:"lastBuyBoxUpdate"`
	LastOffersUpdate               Time    `json:"lastOffersUpdate"`
	LightningDealInfo              any     `json:"lightningDealInfo"`
	Max                            [][]int `json:"max"`
	MaxInInterval                  [][]int `json:"maxInInterval"`
	Min                            [][]int `json:"min"`
	MinInInterval                  [][]int `json:"minInInterval"`
	OfferCountFBA                  int     `json:"offerCountFBA"`
	OfferCountFBM                  int     `json:"offerCountFBM"`
	OutOfStockCountAmazon30        int     `json:"outOfStockCountAmazon30"`
	OutOfStockCountAmazon90        int     `json:"outOfStockCountAmazon90"`
	OutOfStockPercentage180        []int   `json:"outOfStockPercentage180"`
	OutOfStockPercentage30         []int   `json:"outOfStockPercentage30"`
	OutOfStockPercentage365        []int   `json:"outOfStockPercentage365"`
	OutOfStockPercentage90         []int   `json:"outOfStockPercentage90"`
	OutOfStockPercentageInInterval []int   `json:"outOfStockPercentageInInterval"`
	RetrievedOfferCount            int     `json:"retrievedOfferCount"`
	SalesRankDrops180              int     `json:"salesRankDrops180"`
	SalesRankDrops30               int     `json:"salesRankDrops30"`
	SalesRankDrops365              int     `json:"salesRankDrops365"`
	SalesRankDrops90               int     `json:"salesRankDrops90"`
	SellerIDsLowestFBA             any     `json:"sellerIdsLowestFBA"`
	SellerIDsLowestFBM             any     `json:"sellerIdsLowestFBM"`
	TotalOfferCount                int     `json:"totalOfferCount"`
	TradeInPrice                   int     `json:"tradeInPrice"`
}
