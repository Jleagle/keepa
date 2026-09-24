package keepa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type probeResponse struct {
	Envelope
	Value string `json:"value"`
}

// probe runs do against a fake "/probe" endpoint.
func probe(ctx context.Context, c *Client, r request) (*probeResponse, error) {
	if r.path == "" {
		r.path = "/probe"
	}
	if r.query == nil {
		r.query = c.query()
	}
	return do[probeResponse](ctx, c, r)
}

// invalidKeyEnvelope is the real body Keepa returns for a bad key: an
// envelope with an error and all token fields zero.
const invalidKeyEnvelope = `{"error":{"details":"","message":"You used an invalid parameter for this API call.","type":"invalidParameter"},"processingTimeInMs":0,"refillIn":0,"refillRate":0,"timestamp":1790277678673,"tokenFlowReduction":0.0,"tokensConsumed":0,"tokensLeft":0}`

func TestDoSendsGetWithKeyAndDecodes(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope(`"value":"hi"`)))
	res, err := probe(t.Context(), c, request{cost: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Value != "hi" || res.TokensLeft != 1195 || res.RefillRate != 20 {
		t.Errorf("decoded %+v", res)
	}
	last := rec.Last(t)
	if last.method != http.MethodGet || last.path != "/probe" || last.query.Get("key") != "test-key" {
		t.Errorf("request = %+v", last)
	}
	if s := c.Tokens(); !s.Known || s.Left != 1195 || s.RefillRate != 20 || !s.UpdatedAt.Equal(testNow) {
		t.Errorf("bucket after = %+v", s)
	}
}

func TestDoPostsJSONBody(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")))
	if _, err := probe(t.Context(), c, request{cost: 1, body: map[string]any{"page": 2}}); err != nil {
		t.Fatal(err)
	}
	last := rec.Last(t)
	if last.method != http.MethodPost || last.contentType != "application/json" {
		t.Errorf("request = %+v", last)
	}
	var body map[string]any
	if err := json.Unmarshal(last.body, &body); err != nil || body["page"] != float64(2) {
		t.Errorf("body = %s (%v)", last.body, err)
	}
	if last.query.Get("key") != "test-key" {
		t.Error("POST must still carry the key in the query")
	}
}

func TestDoRecordsTokensFromErrorEnvelope(t *testing.T) {
	// A quota-exhausted 429 still carries the real token counts. If they are
	// not recorded, schedulers keep working from the last successful reading.
	body := `{"timestamp":1786000000000,"tokensLeft":0,"tokensConsumed":0,"refillIn":60000,"refillRate":5,"tokenFlowReduction":0,"error":{"type":"notEnoughTokens","message":"You do not have enough tokens","details":""}}`
	var updates []TokenUpdate
	c, _ := newTestClient(t, serveJSON(http.StatusTooManyRequests, body), WithTokenCallback(func(u TokenUpdate) {
		if u.Path != "/token" {
			updates = append(updates, u)
		}
	}))
	_, err := probe(t.Context(), c, request{cost: 1})
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 429 || apiErr.Type != "notEnoughTokens" || apiErr.Message == "" {
		t.Errorf("APIError = %+v", apiErr)
	}
	if !errors.Is(err, ErrNotEnoughTokens) {
		t.Error("should match ErrNotEnoughTokens")
	}
	if len(updates) != 1 || updates[0].Left != 0 || updates[0].RefillRate != 5 || updates[0].Path != "/probe" {
		t.Errorf("updates = %+v", updates)
	}
	if s := c.Tokens(); !s.Known || s.Left != 0 || s.RefillRate != 5 {
		t.Errorf("bucket not updated from the error envelope: %+v", s)
	}
}

func TestDoIgnoresEnvelopeWithoutRefillRate(t *testing.T) {
	// Keepa answers a bad key with an envelope whose token fields are all
	// zero. That is not a bucket reading and must not be recorded.
	called := false
	c, _ := newTestClient(t, serveJSON(http.StatusBadRequest, invalidKeyEnvelope), WithTokenCallback(func(u TokenUpdate) {
		if u.Path != "/token" {
			called = true
		}
	}))
	c.tokens.state = TokenState{Known: true, Left: 1200, RefillRate: 20, UpdatedAt: testNow}
	_, err := probe(t.Context(), c, request{cost: 1})
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok || apiErr.Type != "invalidParameter" || apiErr.StatusCode != 400 {
		t.Fatalf("expected invalidParameter APIError, got %T: %v", err, err)
	}
	if called {
		t.Error("callback fired for an envelope without a refill rate")
	}
	if s := c.Tokens(); s.Left != 1199 || s.RefillRate != 20 {
		t.Errorf("bucket overwritten with zeros: %+v", s)
	}
}

func TestDoIgnoresNonEnvelopeBody(t *testing.T) {
	called := false
	c, _ := newTestClient(t, serveJSON(http.StatusBadGateway, `{"message":"gateway blew up"}`), WithTokenCallback(func(u TokenUpdate) {
		if u.Path != "/token" {
			called = true
		}
	}))
	c.tokens.state = TokenState{Known: true, Left: 1200, RefillRate: 20, UpdatedAt: testNow}
	_, err := probe(t.Context(), c, request{cost: 5})
	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok || httpErr.StatusCode != 502 || !bytes.Contains(httpErr.Body, []byte("gateway")) {
		t.Fatalf("expected *HTTPError 502, got %T: %v", err, err)
	}
	if called {
		t.Error("callback fired for a body that is not a Keepa envelope")
	}
	// The reservation took 5; nothing else may have changed.
	if s := c.Tokens(); s.Left != 1195 || s.RefillRate != 20 {
		t.Errorf("bucket = %+v, want Left 1195", s)
	}
}

func TestDoNon200NonJSON(t *testing.T) {
	c, _ := newTestClient(t, serveJSON(500, "Internal Server Error"))
	_, err := probe(t.Context(), c, request{cost: 1})
	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok || httpErr.StatusCode != 500 {
		t.Fatalf("expected *HTTPError 500, got %T: %v", err, err)
	}
}

func TestDoUndecodable200(t *testing.T) {
	logger, logs := captureLogs()
	c, _ := newTestClient(t, serveJSON(200, `<html>oops</html>`), WithLogger(logger))
	c.tokens.state = TokenState{Known: true, Left: 1200, RefillRate: 20, UpdatedAt: testNow}
	_, err := probe(t.Context(), c, request{cost: 1})
	if err == nil || !strings.Contains(err.Error(), "keepa: decoding /probe response") {
		t.Fatalf("err = %v", err)
	}
	if s := c.Tokens(); s.Left != 1199 {
		t.Errorf("bucket changed beyond the reservation: %+v", s)
	}
	if !strings.Contains(logs.String(), "keepa: undecodable response") {
		t.Errorf("not logged: %s", logs.String())
	}
}

func TestDoWarnsOnCostMismatch(t *testing.T) {
	logger, logs := captureLogs()
	c, _ := newTestClient(t, serveJSON(200, okEnvelope("")), WithLogger(logger)) // reports 1 consumed
	if _, err := probe(t.Context(), c, request{cost: 0}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "keepa: token cost mismatch") {
		t.Errorf("expected a mismatch warning: %s", logs.String())
	}
	logs.Reset()
	if _, err := probe(t.Context(), c, request{cost: 1}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), "mismatch") {
		t.Errorf("no warning expected when actual <= expected: %s", logs.String())
	}
}

func TestDoTokenCallbackFields(t *testing.T) {
	var got TokenUpdate
	c, _ := newTestClient(t, serveJSON(200, okEnvelope("")), WithTokenCallback(func(u TokenUpdate) {
		if u.Path == "/probe" {
			got = u
		}
	}))
	if _, err := probe(t.Context(), c, request{cost: 1}); err != nil {
		t.Fatal(err)
	}
	if got.Left != 1195 || got.Consumed != 1 || got.RefillRate != 20 || got.RefillIn != 30*time.Second || got.FlowReduction != 0 {
		t.Errorf("TokenUpdate = %+v", got)
	}
	if !got.Timestamp.Equal(time.UnixMilli(1790277678673)) {
		t.Errorf("Timestamp = %v", got.Timestamp)
	}
}

func TestDoFallbackTimeoutOnlyWithoutDeadline(t *testing.T) {
	slow := func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(150 * time.Millisecond):
		}
		serveJSON(200, okEnvelope(""))(w, r)
	}
	c, _ := newTestClient(t, slow, WithTimeout(20*time.Millisecond))
	if _, err := probe(context.Background(), c, request{cost: 1}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("without a caller deadline the client timeout applies; got %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := probe(ctx, c, request{cost: 1}); err != nil {
		t.Errorf("a caller deadline must win over the client timeout: %v", err)
	}
	if _, err := probe(context.Background(), c, request{cost: 1, timeout: 2 * time.Second}); err != nil {
		t.Errorf("a per-request timeout must override the client default: %v", err)
	}
}

func TestDoUsesLimiter(t *testing.T) {
	lim := &fakeLimiter{}
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")), WithLimiter(lim))
	if _, err := probe(t.Context(), c, request{cost: 1}); err != nil {
		t.Fatal(err)
	}
	if lim.calls == 0 {
		t.Error("limiter not consulted")
	}
	before := len(rec.Calls())
	lim.err = errors.New("limited")
	if _, err := probe(t.Context(), c, request{cost: 1}); !errors.Is(err, lim.err) {
		t.Errorf("limiter error not returned: %v", err)
	}
	if len(rec.Calls()) != before {
		t.Error("request sent despite the limiter refusing")
	}
}

func TestDoHonoursPerCallReserve(t *testing.T) {
	c, rec := newTestClient(t, serveJSON(200, okEnvelope("")), WithTokenReserve(1000))
	c.tokens.state = TokenState{Known: true, Left: 500, RefillRate: 20, UpdatedAt: testNow}
	_, err := probe(t.Context(), c, request{cost: 5, callParams: callParams{noWait: true}})
	if !errors.Is(err, ErrWouldWait) {
		t.Fatalf("client floor of 1000 should hold the call: %v", err)
	}
	if len(rec.Calls()) != 0 {
		t.Error("request reached the server despite the reserve")
	}
	zero := 0
	if _, err := probe(t.Context(), c, request{cost: 5, callParams: callParams{noWait: true, reserve: &zero}}); err != nil {
		t.Errorf("WithReserve(0) should let the call through: %v", err)
	}
}
