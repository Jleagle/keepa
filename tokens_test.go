package keepa

import (
	"context"
	"errors"
	"net/http"
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
	wait, err := c.reserveTokens(30, 20, false, time.Time{})
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
	wait, err := c.reserveTokens(30, 20, false, time.Time{})
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
	wait, err := c.reserveTokens(10, 20, false, time.Time{})
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
	wait, _ := c.reserveTokens(30, 20, false, time.Time{})
	if wait != 200*time.Second {
		t.Errorf("wait = %v, want 3m20s", wait)
	}
}

func TestReserveTokensQueuesConcurrentCallers(t *testing.T) {
	// At the floor with 20 in hand and 60/min refill, two calls of 60 each:
	// the first waits one minute, the second queues behind it and waits two.
	c := bucketClient(TokenState{Known: true, Left: 20, RefillRate: 60, UpdatedAt: testNow})
	w1, _ := c.reserveTokens(60, 20, false, time.Time{})
	w2, _ := c.reserveTokens(60, 20, false, time.Time{})
	if w1 != time.Minute || w2 != 2*time.Minute {
		t.Errorf("waits = %v, %v; want 1m, 2m", w1, w2)
	}
	if s := c.Tokens(); !s.UpdatedAt.Equal(testNow.Add(2 * time.Minute)) {
		t.Errorf("timeline = %v, want testNow+2m", s.UpdatedAt)
	}
}

func TestReserveTokensWithoutWaiting(t *testing.T) {
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, UpdatedAt: testNow})
	_, err := c.reserveTokens(30, 20, true, time.Time{})
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
	if wait, err := c.reserveTokens(500, 1000, true, time.Time{}); wait != 0 || err != nil {
		t.Errorf("unknown state must run immediately: %v %v", wait, err)
	}
	c = bucketClient(TokenState{Known: true, Left: 0, RefillRate: 10, UpdatedAt: testNow})
	if wait, err := c.reserveTokens(0, 1000, true, time.Time{}); wait != 0 || err != nil {
		t.Errorf("cost 0 must run immediately: %v %v", wait, err)
	}
	if s := c.Tokens(); s.Left != 0 {
		t.Errorf("cost 0 must not change state: %+v", s)
	}
}

func TestRefundTokensRestoresProjection(t *testing.T) {
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, UpdatedAt: testNow})
	before := c.Tokens().Projected(testNow)
	_, err := c.reserveTokens(30, 20, false, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	c.refundTokens(30)
	if after := c.Tokens().Projected(testNow); after != before {
		t.Errorf("projection after refund = %v, want %v", after, before)
	}
}

func TestRefundTokensSkipsWhenNothingReserved(t *testing.T) {
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, UpdatedAt: testNow})
	c.refundTokens(30)
	if s := c.Tokens(); s.Left != 30 || !s.UpdatedAt.Equal(testNow) {
		t.Errorf("refund changed a timeline that was not in the future: %+v", s)
	}
}

func TestRefundTokensSkipsWhenSlotHasPassed(t *testing.T) {
	// If the clock has moved past the reserved slot by cancel time, the
	// tokens are treated as spent and nothing is refunded.
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, UpdatedAt: testNow})
	_, err := c.reserveTokens(30, 20, false, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return testNow.Add(3 * time.Minute) }
	c.refundTokens(30)
	if s := c.Tokens(); s.Left != 20 || !s.UpdatedAt.Equal(testNow.Add(2*time.Minute)) {
		t.Errorf("refund changed a slot that had already passed: %+v", s)
	}
	if p := pendingOf(c); p != 0 {
		t.Errorf("pending = %d, want 0", p)
	}
}

// pendingOf reads the summed cost of sleeping reservations.
func pendingOf(c *Client) int {
	c.tokens.mu.Lock()
	defer c.tokens.mu.Unlock()
	return c.tokens.pending
}

func TestRecordEnvelopeDuringQueueKeepsSlotBalance(t *testing.T) {
	// A 500-token call is queued for T+6m. An envelope at T+6s reports the
	// server's balance, which does not include that spend. The state must
	// describe the balance at the slot end, so a 300-token call at T+5m is
	// still held back instead of over-spending.
	c := bucketClient(TokenState{Known: true, Left: 400, RefillRate: 20, UpdatedAt: testNow})
	if wait, err := c.reserveTokens(500, 20, false, time.Time{}); err != nil || wait != 6*time.Minute {
		t.Fatalf("reserve = %v, %v", wait, err)
	}
	c.now = func() time.Time { return testNow.Add(6 * time.Second) }
	c.recordEnvelope(&Envelope{Timestamp: 1, TokensLeft: 398, RefillRate: 20}, "/product")
	if s := c.Tokens(); s.Left != 16 || !s.UpdatedAt.Equal(testNow.Add(6*time.Minute)) {
		t.Fatalf("state after sync = %+v, want Left 16 at T+6m", s)
	}
	c.now = func() time.Time { return testNow.Add(5 * time.Minute) }
	if _, err := c.reserveTokens(300, 20, true, time.Time{}); !errors.Is(err, ErrWouldWait) {
		t.Errorf("a 300-token call at T+5m must be held back: %v", err)
	}
}

func TestRefundTokensAfterResyncRestoresServerReading(t *testing.T) {
	// Reserve, sync mid-sleep, cancel: the projection must return to what
	// the server reported, because nothing was spent.
	c := bucketClient(TokenState{Known: true, Left: 30, RefillRate: 10, UpdatedAt: testNow})
	if _, err := c.reserveTokens(30, 20, false, time.Time{}); err != nil {
		t.Fatal(err)
	}
	at := testNow.Add(time.Minute)
	c.now = func() time.Time { return at }
	c.recordEnvelope(&Envelope{Timestamp: 1, TokensLeft: 40, RefillRate: 10}, "/probe")
	if s := c.Tokens(); s.Left != 20 || !s.UpdatedAt.Equal(testNow.Add(2*time.Minute)) {
		t.Fatalf("state after sync = %+v, want Left 20 at T+2m", s)
	}
	c.refundTokens(30)
	if got := c.Tokens().Projected(at); got != 40 {
		t.Errorf("projection after refund = %v, want the server's 40", got)
	}
	if p := pendingOf(c); p != 0 {
		t.Errorf("pending = %d, want 0", p)
	}
}

func TestReserveTokensBehindQueueKeepsLaterSlot(t *testing.T) {
	// The queue end is T+2m with 100 tokens at that point, refilling 10/min,
	// so 80 are projected now.
	c := bucketClient(TokenState{Known: true, Left: 100, RefillRate: 10, UpdatedAt: testNow.Add(2 * time.Minute)})

	// Projected 80 covers 30 + 20: the call runs now but must not pull the
	// timeline back.
	if wait, err := c.reserveTokens(30, 20, false, time.Time{}); err != nil || wait != 0 {
		t.Fatalf("reserve = %v, %v", wait, err)
	}
	if s := c.Tokens(); s.Left != 70 || !s.UpdatedAt.Equal(testNow.Add(2*time.Minute)) {
		t.Errorf("state = %+v, want Left 70 at T+2m", s)
	}

	// Projected 50 is short of 40 + 20 by 10: the call waits one minute. Its
	// slot, T+1m, lands before the queue end, so the queue end stays put and
	// the spend comes off the balance there.
	if wait, err := c.reserveTokens(40, 20, false, time.Time{}); err != nil || wait != time.Minute {
		t.Fatalf("reserve = %v, %v", wait, err)
	}
	if s := c.Tokens(); s.Left != 30 || !s.UpdatedAt.Equal(testNow.Add(2*time.Minute)) {
		t.Errorf("state = %+v, want Left 30 at T+2m", s)
	}

	// Projected 10 is short of 60 + 20 by 70: the call waits seven minutes,
	// past the queue end, so the timeline moves out to its slot.
	if wait, err := c.reserveTokens(60, 20, false, time.Time{}); err != nil || wait != 7*time.Minute {
		t.Fatalf("reserve = %v, %v", wait, err)
	}
	if s := c.Tokens(); s.Left != 20 || !s.UpdatedAt.Equal(testNow.Add(7*time.Minute)) {
		t.Errorf("state = %+v, want Left 20 at T+7m", s)
	}
	if p := pendingOf(c); p != 100 {
		t.Errorf("pending = %d, want the two sleeping calls' 100", p)
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
	if p := pendingOf(c); p != 0 {
		t.Errorf("pending = %d after cancel, want 0", p)
	}
}

func TestWaitForTokensReleasesPending(t *testing.T) {
	c := bucketClient(TokenState{Known: true, Left: 19, RefillRate: 6000, UpdatedAt: testNow})
	if err := c.waitForTokens(t.Context(), 1, 20, false); err != nil {
		t.Fatal(err)
	}
	if p := pendingOf(c); p != 0 {
		t.Errorf("pending = %d after the wait elapsed, want 0", p)
	}
}

// A wait the context cannot outlive fails at once with a *TokenWaitError,
// without booking tokens the caller will never spend. Blocking for the whole
// deadline and then failing would hold a consumer slot for the entire budget.
func TestWaitForTokensFailsFastWhenWaitOutlivesDeadline(t *testing.T) {
	now := time.Now()
	c := bucketClient(TokenState{Known: true, Left: 0, RefillRate: 20, UpdatedAt: now})
	c.now = func() time.Time { return now }
	ctx, cancel := context.WithDeadline(t.Context(), now.Add(time.Minute)) // cost 3 + reserve 20 at 20/min is 69s
	defer cancel()

	start := time.Now()
	err := c.waitForTokens(ctx, 3, 20, false)
	werr, ok := errors.AsType[*TokenWaitError](err)
	if !ok {
		t.Fatalf("expected *TokenWaitError, got %T: %v", err, err)
	}
	if werr.Wait != 69*time.Second || werr.Cost != 3 {
		t.Errorf("TokenWaitError = %+v", werr)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("blocked for %s before failing", elapsed)
	}
	if s := c.Tokens(); s.Left != 0 || !s.UpdatedAt.Equal(now) {
		t.Errorf("failed wait changed the bucket: %+v", s)
	}
	c.tokens.mu.Lock()
	defer c.tokens.mu.Unlock()
	if c.tokens.pending != 0 {
		t.Errorf("pending = %d after a failed wait, want 0", c.tokens.pending)
	}
}

// A wait that fits inside the deadline proceeds as normal.
func TestWaitForTokensWithinDeadlineSleeps(t *testing.T) {
	now := time.Now()
	c := bucketClient(TokenState{Known: true, Left: 19, RefillRate: 6000, UpdatedAt: now}) // 2 tokens short: 20ms
	c.now = func() time.Time { return now }
	ctx, cancel := context.WithDeadline(t.Context(), now.Add(time.Second))
	defer cancel()
	if err := c.waitForTokens(ctx, 1, 20, false); err != nil {
		t.Fatalf("a wait within the deadline must succeed: %v", err)
	}
}

// WithMaxTokenWait bounds the wait even when the context has no deadline,
// and the tighter of the two bounds wins.
func TestWaitForTokensHonoursMaxTokenWait(t *testing.T) {
	now := time.Now()
	c := bucketClient(TokenState{Known: true, Left: 0, RefillRate: 20, UpdatedAt: now}, WithMaxTokenWait(time.Minute))
	c.now = func() time.Time { return now }

	err := c.waitForTokens(t.Context(), 3, 20, false) // 69 seconds, no context deadline
	if !errors.Is(err, ErrWouldWait) {
		t.Fatalf("err = %v, want ErrWouldWait from the client cap", err)
	}
	c.tokens.mu.Lock()
	pending := c.tokens.pending
	c.tokens.mu.Unlock()
	if pending != 0 {
		t.Errorf("pending = %d after a capped wait, want 0", pending)
	}

	// A generous context deadline does not loosen the cap.
	ctx, cancel := context.WithDeadline(t.Context(), now.Add(time.Hour))
	defer cancel()
	if err := c.waitForTokens(ctx, 3, 20, false); !errors.Is(err, ErrWouldWait) {
		t.Errorf("err = %v, want ErrWouldWait: the client cap is tighter than the context", err)
	}

	// A wait inside the cap proceeds.
	c.tokens.state = TokenState{Known: true, Left: 19, RefillRate: 6000, UpdatedAt: now} // 20ms
	if err := c.waitForTokens(t.Context(), 1, 20, false); err != nil {
		t.Errorf("a wait inside the cap must succeed: %v", err)
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
	if s := c.Tokens(); !s.Known || s.Left != 1200 {
		t.Fatalf("bucket = %+v, want Known at 1200", s)
	}
	// Now known and 3800 below the floor, the free call still goes through.
	if _, err := c.GetTokenStatus(t.Context()); err != nil {
		t.Errorf("a free call below the floor must not be held: %v", err)
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

func TestSeedConcurrentCallersWaitForSeed(t *testing.T) {
	// Ten first calls on a fresh client with a floor equal to the seeded
	// balance: every one of them must be held back, which is only possible
	// if the callers that did not seed waited for the seed to finish.
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")), WithTokenReserve(1200))
	rec.SetTokenHandler(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(30 * time.Millisecond)
		serveJSON(200, tokenFixture)(w, r)
	})
	errs := make(chan error, 10)
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			_, err := probe(t.Context(), c, request{cost: 5, callParams: callParams{noWait: true}})
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, ErrWouldWait) {
			t.Errorf("a caller bypassed the reserve: %v", err)
		}
	}
	if got := rec.Paths(); !slices.Equal(got, []string{"/token"}) {
		t.Errorf("paths = %v, want exactly one seed and no probes", got)
	}
}

func TestSeedWaiterHonoursOwnContext(t *testing.T) {
	// A caller waiting on someone else's seed gives up when its own context
	// expires rather than waiting for the seed to finish.
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	rec.SetTokenHandler(func(w http.ResponseWriter, r *http.Request) {
		select {
		case arrived <- struct{}{}:
		default:
		}
		<-release
		serveJSON(200, tokenFixture)(w, r)
	})
	var wg sync.WaitGroup
	wg.Go(func() { _, _ = probe(t.Context(), c, request{cost: 1}) })
	<-arrived // the seed is now blocked inside the token handler

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	_, err := probe(ctx, c, request{cost: 1})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("waiter should fail with its own deadline, got %v", err)
	}
	close(release)
	wg.Wait()
}

func TestSeedSurvivesInitiatorCancellation(t *testing.T) {
	// The caller that starts the seed gives up; the seed carries on and
	// still syncs the bucket for everyone else.
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	rec.SetTokenHandler(func(w http.ResponseWriter, r *http.Request) {
		select {
		case arrived <- struct{}{}:
		default:
		}
		<-release
		serveJSON(200, tokenFixture)(w, r)
	})

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	_, err := probe(ctx, c, request{cost: 1})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("initiator should fail with its own deadline, got %v", err)
	}
	<-arrived
	close(release)

	deadline := time.Now().Add(time.Second)
	for !c.Tokens().Known && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !c.Tokens().Known {
		t.Fatal("the seed did not complete after its initiator was cancelled")
	}
	if got := rec.Paths(); !slices.Equal(got, []string{"/token"}) {
		t.Errorf("paths = %v, want only the seed", got)
	}
}
