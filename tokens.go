package keepa

import (
	"context"
	"sync"
	"time"
)

// TokenState is a snapshot of the client's view of the Keepa token bucket.
type TokenState struct {
	Known         bool      // false until an envelope or seed has been recorded
	Left          int       // tokens in the bucket at UpdatedAt
	RefillRate    int       // tokens per minute granted by the plan
	FlowReduction float64   // tokens per minute lost to tracking subscriptions
	UpdatedAt     time.Time // when Left was true; in the future while calls are queued
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
	seeding bool

	// generation counts every time the state is re-synced from the server.
	// Nothing in this file increments it; Task 6's recordEnvelope bumps it
	// under the lock whenever an envelope overwrites the queued timeline, so
	// a cancelled reservation can tell whether its slot is still the one it
	// reserved before deciding to refund it.
	generation uint64
}

// Tokens returns a snapshot of the client's view of the token bucket.
func (c *Client) Tokens() TokenState {
	c.tokens.mu.Lock()
	defer c.tokens.mu.Unlock()
	return c.tokens.state
}

// reserveTokens applies the wait rule. It returns how long the caller must
// wait before running (0 to run now), the bucket generation observed while
// reserving (for a later refundTokens call), and, with noWait, a
// *TokenWaitError instead of reserving a slot.
func (c *Client) reserveTokens(cost, reserve int, noWait bool) (wait time.Duration, gen uint64, err error) {
	c.tokens.mu.Lock()
	defer c.tokens.mu.Unlock()

	gen = c.tokens.generation
	s := &c.tokens.state
	if !s.Known || cost <= 0 {
		return 0, gen, nil
	}
	now := c.now()
	projected := s.Projected(now)
	target := float64(cost + reserve)
	if projected >= target {
		s.Left = int(projected) - cost
		s.UpdatedAt = now
		return 0, gen, nil
	}
	wait = time.Duration((target - projected) / s.NetRefillRate() * float64(time.Minute))
	if noWait {
		return wait, gen, &TokenWaitError{Wait: wait, Cost: cost, Reserve: reserve, Projected: projected}
	}
	s.Left = reserve
	s.UpdatedAt = now.Add(wait)
	return wait, gen, nil
}

// refundTokens releases a slot whose call was cancelled before it ran. gen is
// the generation reserveTokens observed when it reserved the slot.
//
// The refund is applied only if both hold at cancel time: the bucket has not
// been re-synced from the server since the reservation (s.generation == gen)
// and the reserved slot is still in the future (s.UpdatedAt.After(c.now())).
// Either condition failing means the state under UpdatedAt is no longer the
// timeline this call reserved, so nothing is refunded. A skipped refund only
// under-spends the bucket until the next envelope corrects it; a wrong
// refund over-credits it and risks a 429.
func (c *Client) refundTokens(cost int, gen uint64) {
	c.tokens.mu.Lock()
	defer c.tokens.mu.Unlock()

	if c.tokens.generation != gen {
		return
	}
	s := &c.tokens.state
	if s.UpdatedAt.After(c.now()) {
		refund := time.Duration(float64(cost) / s.NetRefillRate() * float64(time.Minute))
		s.UpdatedAt = s.UpdatedAt.Add(-refund)
	}
}

// waitForTokens blocks until the bucket can pay cost while keeping reserve.
func (c *Client) waitForTokens(ctx context.Context, cost, reserve int, noWait bool) error {
	wait, gen, err := c.reserveTokens(cost, reserve, noWait)
	if err != nil || wait <= 0 {
		return err
	}
	c.logger.Info("keepa: waiting for tokens", "cost", cost, "reserve", reserve, "wait", wait)

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		c.refundTokens(cost, gen)
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// recordEnvelope syncs the bucket with an envelope and notifies the callback.
// It bumps the bucket generation so a queued call cancelled after this point
// does not refund its slot against the fresh reading.
func (c *Client) recordEnvelope(env *Envelope, path string) {
	now := c.now()
	c.tokens.mu.Lock()
	s := &c.tokens.state
	s.Known = true
	s.Left = env.TokensLeft
	s.RefillRate = env.RefillRate
	s.FlowReduction = env.TokenFlowReduction
	if now.After(s.UpdatedAt) {
		s.UpdatedAt = now
	}
	c.tokens.generation++
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
// paid call. Concurrent callers do not wait for a seed already in flight; a
// failed seed is logged and retried on the next paid call.
func (c *Client) seed(ctx context.Context) {
	c.tokens.mu.Lock()
	if c.tokens.state.Known || c.tokens.seeding {
		c.tokens.mu.Unlock()
		return
	}
	c.tokens.seeding = true
	c.tokens.mu.Unlock()

	_, err := c.GetTokenStatus(ctx)

	c.tokens.mu.Lock()
	c.tokens.seeding = false
	c.tokens.mu.Unlock()

	if err != nil {
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
