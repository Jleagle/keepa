package keepa

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// testNow pins the client clock in every test that needs determinism.
var testNow = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

func bucketClient(state TokenState, opts ...Option) *Client {
	c := NewClient("key", opts...)
	c.now = func() time.Time { return testNow }
	c.tokens.state = state
	return c
}

func TestTokenStateProjected(t *testing.T) {
	s := TokenState{Known: true, Left: 100, RefillRate: 10, FlowReduction: 2.5, UpdatedAt: testNow}
	if got := s.NetRefillRate(); got != 7.5 {
		t.Errorf("NetRefillRate() = %v, want 7.5", got)
	}
	if got := s.Projected(testNow.Add(4 * time.Minute)); got != 130 {
		t.Errorf("Projected(+4m) = %v, want 130", got)
	}
	if got := s.Projected(testNow.Add(-2 * time.Minute)); got != 85 {
		t.Errorf("Projected(-2m) = %v, want 85", got)
	}
	if got := (TokenState{}).Projected(testNow); got != 0 {
		t.Errorf("unknown state Projected() = %v, want 0", got)
	}
	if got := (TokenState{RefillRate: 3, FlowReduction: 5}).NetRefillRate(); got != 1 {
		t.Errorf("NetRefillRate() floor = %v, want 1", got)
	}
}

func TestReserveTokensRunsWhenPlentiful(t *testing.T) {
	c := bucketClient(TokenState{Known: true, Left: 100, RefillRate: 10, UpdatedAt: testNow})
	wait, _, err := c.reserveTokens(30, 20, false)
	if err != nil || wait != 0 {
		t.Fatalf("reserveTokens = %v, %v; want 0, nil", wait, err)
	}
	if s := c.Tokens(); s.Left != 70 || !s.UpdatedAt.Equal(testNow) {
		t.Errorf("state after = %+v, want Left 70 at testNow", s)
	}
}

func TestReserveTokensWaitsToKeepReserve(t *testing.T) {
	// 30 in hand, need 30 + 20 reserve = 50, refill 10/min: two minutes.
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, UpdatedAt: testNow})
	wait, _, err := c.reserveTokens(30, 20, false)
	if err != nil || wait != 2*time.Minute {
		t.Fatalf("reserveTokens = %v, %v; want 2m, nil", wait, err)
	}
	if s := c.Tokens(); s.Left != 20 || !s.UpdatedAt.Equal(testNow.Add(2*time.Minute)) {
		t.Errorf("state after = %+v, want Left 20 at testNow+2m", s)
	}
}

func TestReserveTokensProjectsRefill(t *testing.T) {
	// Seen empty three minutes ago at 10/min: 30 projected, which covers cost 10 + reserve 20 exactly.
	c := bucketClient(TokenState{Known: true, Left: 0, RefillRate: 10, UpdatedAt: testNow.Add(-3 * time.Minute)})
	wait, _, err := c.reserveTokens(10, 20, false)
	if err != nil || wait != 0 {
		t.Fatalf("reserveTokens = %v, %v; want 0, nil", wait, err)
	}
	if s := c.Tokens(); s.Left != 20 || !s.UpdatedAt.Equal(testNow) {
		t.Errorf("state after = %+v, want Left 20 at testNow", s)
	}
}

func TestReserveTokensUsesNetRefillRate(t *testing.T) {
	// 10/min minus 4/min tracking: 6/min. 20 short at 6/min is 200 seconds.
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, FlowReduction: 4, UpdatedAt: testNow})
	wait, _, _ := c.reserveTokens(30, 20, false)
	if wait != 200*time.Second {
		t.Errorf("wait = %v, want 3m20s", wait)
	}
}

func TestReserveTokensQueuesConcurrentCallers(t *testing.T) {
	// At the floor with 20 in hand and 60/min refill, two calls of 60 each:
	// the first waits one minute, the second queues behind it and waits two.
	c := bucketClient(TokenState{Known: true, Left: 20, RefillRate: 60, UpdatedAt: testNow})
	w1, _, _ := c.reserveTokens(60, 20, false)
	w2, _, _ := c.reserveTokens(60, 20, false)
	if w1 != time.Minute || w2 != 2*time.Minute {
		t.Errorf("waits = %v, %v; want 1m, 2m", w1, w2)
	}
	if s := c.Tokens(); !s.UpdatedAt.Equal(testNow.Add(2 * time.Minute)) {
		t.Errorf("timeline = %v, want testNow+2m", s.UpdatedAt)
	}
}

func TestReserveTokensWithoutWaiting(t *testing.T) {
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, UpdatedAt: testNow})
	_, _, err := c.reserveTokens(30, 20, true)
	werr, ok := errors.AsType[*TokenWaitError](err)
	if !ok {
		t.Fatalf("expected *TokenWaitError, got %T: %v", err, err)
	}
	if werr.Wait != 2*time.Minute || werr.Cost != 30 || werr.Reserve != 20 || werr.Projected != 30 {
		t.Errorf("TokenWaitError = %+v", werr)
	}
	if !errors.Is(err, ErrWouldWait) {
		t.Error("should match ErrWouldWait")
	}
	if s := c.Tokens(); s.Left != 30 || !s.UpdatedAt.Equal(testNow) {
		t.Errorf("non-blocking mode must not reserve: %+v", s)
	}
}

func TestReserveTokensUnknownStateOrZeroCost(t *testing.T) {
	c := bucketClient(TokenState{})
	if wait, _, err := c.reserveTokens(500, 1000, true); wait != 0 || err != nil {
		t.Errorf("unknown state must run immediately: %v %v", wait, err)
	}
	c = bucketClient(TokenState{Known: true, Left: 0, RefillRate: 10, UpdatedAt: testNow})
	if wait, _, err := c.reserveTokens(0, 1000, true); wait != 0 || err != nil {
		t.Errorf("cost 0 must run immediately: %v %v", wait, err)
	}
	if s := c.Tokens(); s.Left != 0 {
		t.Errorf("cost 0 must not change state: %+v", s)
	}
}

func TestRefundTokensRestoresProjection(t *testing.T) {
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, UpdatedAt: testNow})
	before := c.Tokens().Projected(testNow)
	_, gen, err := c.reserveTokens(30, 20, false)
	if err != nil {
		t.Fatal(err)
	}
	c.refundTokens(30, gen)
	if after := c.Tokens().Projected(testNow); after != before {
		t.Errorf("projection after refund = %v, want %v", after, before)
	}
}

func TestRefundTokensSkipsWhenNothingReserved(t *testing.T) {
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, UpdatedAt: testNow})
	c.refundTokens(30, c.tokens.generation)
	if s := c.Tokens(); !s.UpdatedAt.Equal(testNow) {
		t.Errorf("refund moved a timeline that was not in the future: %v", s.UpdatedAt)
	}
}

func TestRefundTokensSkipsAfterResync(t *testing.T) {
	// An envelope arriving during the sleep bumps the generation; the
	// cancelled call must not refund against the server's fresh reading.
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, UpdatedAt: testNow})
	_, gen, err := c.reserveTokens(30, 20, false)
	if err != nil {
		t.Fatal(err)
	}
	c.tokens.mu.Lock()
	c.tokens.generation++
	c.tokens.state.Left = 500 // what an envelope would write
	c.tokens.mu.Unlock()
	c.refundTokens(30, gen)
	if s := c.Tokens(); s.Left != 500 || !s.UpdatedAt.Equal(testNow.Add(2*time.Minute)) {
		t.Errorf("refund applied after a resync: %+v", s)
	}
}

func TestRefundTokensSkipsWhenSlotHasPassed(t *testing.T) {
	// If the clock has moved past the reserved slot by cancel time, the
	// tokens are treated as spent and nothing is refunded.
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, UpdatedAt: testNow})
	_, gen, err := c.reserveTokens(30, 20, false)
	if err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return testNow.Add(3 * time.Minute) }
	c.refundTokens(30, gen)
	if s := c.Tokens(); !s.UpdatedAt.Equal(testNow.Add(2 * time.Minute)) {
		t.Errorf("refund moved a slot that had already passed: %v", s.UpdatedAt)
	}
}

func TestWaitForTokensSleepsThenRuns(t *testing.T) {
	// Two tokens short at 6000/min is 20ms.
	logger, logs := captureLogs()
	c := bucketClient(TokenState{Known: true, Left: 19, RefillRate: 6000, UpdatedAt: testNow}, WithLogger(logger))
	start := time.Now()
	if err := c.waitForTokens(t.Context(), 1, 20, false); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
		t.Errorf("returned after %v, expected a wait of about 20ms", elapsed)
	}
	if !strings.Contains(logs.String(), "keepa: waiting for tokens") {
		t.Errorf("wait not logged: %s", logs.String())
	}
}

func TestWaitForTokensCancelRefunds(t *testing.T) {
	// Empty bucket at 1/min: a cost of 1 with reserve 20 would wait 21 minutes.
	c := bucketClient(TokenState{Known: true, Left: 0, RefillRate: 1, UpdatedAt: testNow})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := c.waitForTokens(ctx, 1, 20, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if got := c.Tokens().Projected(testNow); got != 0 {
		t.Errorf("projection after cancel = %v, want 0 (slot refunded)", got)
	}
}

func TestWaitForTokensNoWaitDoesNotLog(t *testing.T) {
	logger, logs := captureLogs()
	c := bucketClient(TokenState{Known: true, Left: 0, RefillRate: 1, UpdatedAt: testNow}, WithLogger(logger))
	if err := c.waitForTokens(t.Context(), 1, 20, true); !errors.Is(err, ErrWouldWait) {
		t.Fatalf("err = %v", err)
	}
	if logs.Len() != 0 {
		t.Errorf("non-blocking mode logged: %s", logs.String())
	}
}

func TestSeedRunsBeforeFirstPaidCallOnly(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	for range 2 {
		if _, err := probe(t.Context(), c, request{cost: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if got := rec.Paths(); !slices.Equal(got, []string{"/token", "/probe", "/probe"}) {
		t.Errorf("paths = %v, want one seed then two probes", got)
	}
}

func TestSeedAppliesReserveToFirstCall(t *testing.T) {
	// The fixture reports 1200 tokens. With a floor of 1200 the very first
	// paid call is already held back, and no probe reaches the server.
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")), WithTokenReserve(1200))
	_, err := probe(t.Context(), c, request{cost: 5, callParams: callParams{noWait: true}})
	if !errors.Is(err, ErrWouldWait) {
		t.Fatalf("err = %v, want ErrWouldWait", err)
	}
	if got := costOf(t, err); got != 5 {
		t.Errorf("cost = %d, want 5", got)
	}
	if got := rec.Paths(); !slices.Equal(got, []string{"/token"}) {
		t.Errorf("paths = %v, want only the seed", got)
	}
}

func TestSeedFailureIsLoggedAndIgnored(t *testing.T) {
	logger, logs := captureLogs()
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")), WithLogger(logger))
	rec.SetTokenHandler(serveJSON(500, "boom"))
	if _, err := probe(t.Context(), c, request{cost: 1}); err != nil {
		t.Fatalf("the paid call must proceed when the seed fails: %v", err)
	}
	if !strings.Contains(logs.String(), "keepa: token seed failed") {
		t.Errorf("seed failure not logged: %s", logs.String())
	}
	// The probe's envelope synced the bucket, so no further seed is needed.
	if _, err := probe(t.Context(), c, request{cost: 1}); err != nil {
		t.Fatal(err)
	}
	if got := rec.Paths(); !slices.Equal(got, []string{"/token", "/probe", "/probe"}) {
		t.Errorf("paths = %v", got)
	}
}

func TestSeedRetriesWhileStateUnknown(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(502, `{"message":"down"}`))
	rec.SetTokenHandler(serveJSON(500, "boom"))
	for range 2 {
		_, _ = probe(t.Context(), c, request{cost: 1})
	}
	if got := rec.Paths(); !slices.Equal(got, []string{"/token", "/probe", "/token", "/probe"}) {
		t.Errorf("paths = %v, want a seed attempt before each call while unknown", got)
	}
}

func TestSeedDoesNotRecurse(t *testing.T) {
	// GetTokenStatus costs nothing, so it never triggers a seed of its own.
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	res, err := c.GetTokenStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.TokensLeft != 1200 || res.RefillRate != 20 {
		t.Errorf("TokenResponse = %+v", res.Envelope)
	}
	if got := rec.Paths(); !slices.Equal(got, []string{"/token"}) {
		t.Errorf("paths = %v, want exactly one /token", got)
	}
	if s := c.Tokens(); !s.Known || s.Left != 1200 || s.RefillRate != 20 {
		t.Errorf("bucket = %+v", s)
	}
	if got := rec.Calls()[0].query.Get("key"); got != "test-key" {
		t.Errorf("key = %q", got)
	}
}

func TestSeedBypassesReserve(t *testing.T) {
	// The free status call goes through even when the bucket is below the floor.
	c, _ := newTestClient(t, serveJSON(200, okEnvelope("")), WithTokenReserve(5000))
	if _, err := c.GetTokenStatus(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := probe(t.Context(), c, request{cost: 1, callParams: callParams{noWait: true}}); !errors.Is(err, ErrWouldWait) {
		t.Errorf("a paid call should be held by the floor: %v", err)
	}
}

func TestSeedRunsOnceUnderConcurrency(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { _, _ = probe(t.Context(), c, request{cost: 1}) })
	}
	wg.Wait()
	seeds := 0
	for _, p := range rec.Paths() {
		if p == "/token" {
			seeds++
		}
	}
	if seeds != 1 {
		t.Errorf("seeded %d times, want 1", seeds)
	}
}
