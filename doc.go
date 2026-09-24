// Package keepa is a Go client for the Keepa API, https://keepa.com/api-docs/.
//
// Create a client with NewClient and call the endpoint methods. Every method
// takes a context first, then the arguments Keepa requires, then options:
//
//	client := keepa.NewClient(apiKey, keepa.WithTokenReserve(1000))
//	resp, err := client.GetProducts(ctx, keepa.DomainUS, []string{"B07XJ8C8F5"},
//		keepa.WithStats(time.Now().AddDate(0, -1, 0)), keepa.WithRatings())
//
// The client tracks Keepa's token bucket between responses and blocks a call
// that would drop the bucket below the reserve. See Client.Tokens,
// WithTokenReserve, WithReserve and WithoutWaiting.
package keepa
