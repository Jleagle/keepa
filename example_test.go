package keepa_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Jleagle/keepa"
)

func ExampleNewClient() {
	client := keepa.NewClient("your-api-key",
		keepa.WithTokenReserve(1000),
		keepa.WithTokenCallback(func(u keepa.TokenUpdate) {
			log.Printf("%s consumed %d tokens, %d left", u.Path, u.Consumed, u.Left)
		}),
	)

	ctx := context.Background()
	resp, err := client.GetProducts(ctx, keepa.DomainUS, []string{"B07XJ8C8F5"},
		keepa.WithStats(time.Now().AddDate(0, -1, 0)),
		keepa.WithRatings(),
	)
	if err != nil {
		var apiErr *keepa.APIError
		switch {
		case errors.Is(err, keepa.ErrNotEnoughTokens):
			log.Print("quota exhausted")
		case errors.As(err, &apiErr):
			log.Printf("keepa said %s: %s", apiErr.Type, apiErr.Message)
		default:
			log.Print(err)
		}
		return
	}

	for _, p := range resp.Products {
		fmt.Println(p.ASIN, p.Title, p.LastUpdate.Time())
		for at, price := range p.CSV.Amazon.Pairs() {
			if price >= 0 {
				fmt.Println(at.Time(), price)
			}
		}
	}
}

func ExampleWithoutWaiting() {
	client := keepa.NewClient("your-api-key", keepa.WithTokenReserve(1000))

	// A queue consumer that would rather re-queue than block.
	_, err := client.GetLightningDeals(context.Background(), keepa.DomainUS, keepa.WithoutWaiting())
	var wait *keepa.TokenWaitError
	if errors.As(err, &wait) {
		fmt.Println("retry in", wait.Wait)
	}

	// An interactive call that may dip below the floor.
	_, _ = client.SearchProducts(context.Background(), keepa.DomainUS, "usb c cable",
		keepa.WithASINsOnly(), keepa.WithReserve(0))
}
