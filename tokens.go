package keepa

import (
	"context"
	"sync"
	"time"
)

// TokenState is a snapshot of the client's view of the Keepa token bucket.
type TokenState struct {
	Known         bool    // false until an envelope or seed has been recorded
	Left          int     // tokens in the bucket at UpdatedAt
	RefillRate    int     // tokens per minute granted by the plan
	FlowReduction float64 // tokens per minute lost to tracking subscriptions
	// UpdatedAt is when Left is true. It is in the future while calls are
	// queued, in which case Left is the balance at that slot end after the
	// queued spends.
	UpdatedAt time.Time
}

// NetRefillRate is the effective refill in tokens per minute, never below 1.
func (s TokenState) NetRefillRate() float64 {
	net := float64(s.RefillRate) - s.FlowReduction
	if net <= 0 {
		return 1
	}
	return net
}

// Projected estimates the tokens available at the given time. Times before
// UpdatedAt project downwards, which is how queued calls reserve their slots.
func (s TokenState) Projected(at time.Time) float64 {
	if !s.Known {
		return 0
	}
	return float64(s.Left) + at.Sub(s.UpdatedAt).Minutes()*s.NetRefillRate()
}

// TokenUpdate is passed to the callback registered with WithTokenCallback.
type TokenUpdate struct {
	Left          int
	Consumed      int
	RefillRate    int
	RefillIn      time.Duration
	FlowReduction float64
	Path          string    // request path, such as "/product"
	Timestamp     time.Time // server time from the envelope
}

type tokenBucket struct {
	mu      sync.Mutex
	state   TokenState
	seeding chan struct{} // non-nil while a seed is in flight; closed when it finishes
	pending int           // summed cost of reservations that are still sleeping
}

// Tokens returns a snapshot of the client's view of the token bucket.
func (c *Client) Tokens() TokenState {
	c.tokens.mu.Lock()
	defer c.tokens.mu.Unlock()
	return c.tokens.state
}

// reserveTokens applies the wait rule. It returns how long the caller must
// wait before running (0 to run now) and, with noWait, a *TokenWaitError
// instead of reserving a slot. A reservation that has to wait is counted in
// pending until releaseTokens or refundTokens settles it.
func (c *Client) reserveTokens(cost, reserve int, noWait bool) (time.Duration, error) {
	c.tokens.mu.Lock()
	defer c.tokens.mu.Unlock()

	s := &c.tokens.state
	if !s.Known || cost <= 0 {
		return 0, nil
	}
	now := c.now()
	projected := s.Projected(now)
	target := float64(cost + reserve)
	if projected >= target {
		if s.UpdatedAt.After(now) {
			s.Left -= cost // the timeline end stays where the queue put it
		} else {
			s.Left = int(projected) - cost
			s.UpdatedAt = now
		}
		return 0, nil
	}
	wait := time.Duration((target - projected) / s.NetRefillRate() * float64(time.Minute))
	if noWait {
		return wait, &TokenWaitError{Wait: wait, Cost: cost, Reserve: reserve, Projected: projected}
	}
	if runAt := now.Add(wait); runAt.After(s.UpdatedAt) {
		s.Left = reserve
		s.UpdatedAt = runAt
	} else {
		s.Left -= cost
	}
	c.tokens.pending += cost
	return wait, nil
}

// releaseTokens marks a sleeping reservation as committed once its wait has
// elapsed and the request is about to be sent.
func (c *Client) releaseTokens(cost int) {
	c.tokens.mu.Lock()
	c.tokens.pending -= cost
	c.tokens.mu.Unlock()
}

// refundTokens returns a reservation whose call was cancelled before it ran.
// The refund applies only while the slot is still in the future; once the
// slot has passed the tokens are treated as spent.
func (c *Client) refundTokens(cost int) {
	c.tokens.mu.Lock()
	defer c.tokens.mu.Unlock()
	c.tokens.pending -= cost
	if c.tokens.state.UpdatedAt.After(c.now()) {
		c.tokens.state.Left += cost
	}
}

// waitForTokens blocks until the bucket can pay cost while keeping reserve.
func (c *Client) waitForTokens(ctx context.Context, cost, reserve int, noWait bool) error {
	wait, err := c.reserveTokens(cost, reserve, noWait)
	if err != nil || wait <= 0 {
		return err
	}
	c.logger.Info("keepa: waiting for tokens", "cost", cost, "reserve", reserve, "wait", wait)

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		c.refundTokens(cost)
		return ctx.Err()
	case <-timer.C:
		c.releaseTokens(cost)
		return nil
	}
}

// recordEnvelope syncs the bucket with an envelope and notifies the callback.
//
// When calls are queued, UpdatedAt is in the future and Left is the balance
// at that slot end after the queued spends. The server's reading does not
// include the spends of calls still sleeping, so they are subtracted again
// (pending) and the reading is projected forward to the slot end. One
// approximation remains: requests already sent whose envelope has not yet
// returned are not subtracted; the next envelope corrects it.
func (c *Client) recordEnvelope(env *Envelope, path string) {
	now := c.now()
	c.tokens.mu.Lock()
	s := &c.tokens.state
	s.Known = true
	s.RefillRate = env.RefillRate
	s.FlowReduction = env.TokenFlowReduction
	if s.UpdatedAt.After(now) {
		// Calls are queued: the server's balance does not include their
		// spends yet, and the state describes the balance at the slot end.
		refill := s.UpdatedAt.Sub(now).Minutes() * s.NetRefillRate()
		s.Left = env.TokensLeft - c.tokens.pending + int(refill)
	} else {
		s.Left = env.TokensLeft
		s.UpdatedAt = now
	}
	c.tokens.mu.Unlock()

	if c.onTokens != nil {
		c.onTokens(TokenUpdate{
			Left:          env.TokensLeft,
			Consumed:      env.TokensConsumed,
			RefillRate:    env.RefillRate,
			RefillIn:      time.Duration(env.RefillIn) * time.Millisecond,
			FlowReduction: env.TokenFlowReduction,
			Path:          path,
			Timestamp:     time.UnixMilli(env.Timestamp),
		})
	}
}

// seed fetches the token status once so the reserve applies to the first
// paid call. The first caller starts the fetch in its own goroutine, detached
// from its cancellation, so one caller giving up does not fail the seed for
// the others; every caller then waits for it, bounded by its own context. A
// failed seed is logged and retried on the next paid call while the bucket
// is still unknown.
func (c *Client) seed(ctx context.Context) {
	c.tokens.mu.Lock()
	if c.tokens.state.Known {
		c.tokens.mu.Unlock()
		return
	}
	done := c.tokens.seeding
	if done == nil {
		done = make(chan struct{})
		c.tokens.seeding = done
		go c.runSeed(context.WithoutCancel(ctx), done)
	}
	c.tokens.mu.Unlock()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

// runSeed fetches the token status, then wakes every caller waiting on done.
func (c *Client) runSeed(ctx context.Context, done chan struct{}) {
	defer func() {
		c.tokens.mu.Lock()
		c.tokens.seeding = nil
		c.tokens.mu.Unlock()
		close(done)
	}()
	if _, err := c.GetTokenStatus(ctx); err != nil {
		c.logger.Warn("keepa: token seed failed", "error", err)
	}
}

// TokenResponse is returned by GetTokenStatus. Only the Envelope is populated.
type TokenResponse struct {
	Envelope
}

// GetTokenStatus retrieves the token bucket state. Cost: 0 tokens, so it is
// never held back by the reserve.
func (c *Client) GetTokenStatus(ctx context.Context) (*TokenResponse, error) {
	return do[TokenResponse](ctx, c, request{path: "/token", query: c.query(), cost: 0})
}
