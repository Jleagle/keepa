package keepa

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

const envelopeFields = `"timestamp":1790277678673,"tokensLeft":1195,"tokensConsumed":1,"refillIn":30000,"refillRate":20,"tokenFlowReduction":0,"processingTimeInMs":12`

// tokenFixture answers GET /token: 1200 tokens in hand, 20 per minute.
const tokenFixture = `{"timestamp":1790277678673,"tokensLeft":1200,"tokensConsumed":0,"refillIn":30000,"refillRate":20,"tokenFlowReduction":0,"processingTimeInMs":1}`

// okEnvelope wraps a payload fragment such as `"products":[]` in a 200 envelope
// that reports one token consumed and 1195 left.
func okEnvelope(payload string) string {
	if payload == "" {
		return "{" + envelopeFields + "}"
	}
	return "{" + envelopeFields + "," + payload + "}"
}

type recorded struct {
	method      string
	path        string
	query       url.Values
	contentType string
	body        []byte
}

type recorder struct {
	mu           sync.Mutex
	calls        []recorded
	tokenHandler http.HandlerFunc
}

func (r *recorder) Calls() []recorded {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

// Paths returns every request path in order, including the token seed.
func (r *recorder) Paths() []string {
	var paths []string
	for _, c := range r.Calls() {
		paths = append(paths, c.path)
	}
	return paths
}

// Last returns the most recent request that was not the token seed.
func (r *recorder) Last(t *testing.T) recorded {
	t.Helper()
	calls := r.Calls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].path != "/token" {
			return calls[i]
		}
	}
	t.Fatal("no request reached the endpoint")
	return recorded{}
}

// SetTokenHandler replaces the default /token response.
func (r *recorder) SetTokenHandler(h http.HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokenHandler = h
}

// newTestClient starts a server that records every request, answers GET /token
// with tokenFixture and everything else with handler, and returns a client
// pointed at it with the clock pinned to testNow.
func newTestClient(t *testing.T, handler http.HandlerFunc, opts ...Option) (*Client, *recorder) {
	t.Helper()
	rec := &recorder{tokenHandler: serveJSON(http.StatusOK, tokenFixture)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec.mu.Lock()
		rec.calls = append(rec.calls, recorded{
			method:      r.Method,
			path:        r.URL.Path,
			query:       r.URL.Query(),
			contentType: r.Header.Get("Content-Type"),
			body:        body,
		})
		tokenHandler := rec.tokenHandler
		rec.mu.Unlock()
		r.Body = io.NopCloser(bytes.NewReader(body))
		if r.URL.Path == "/token" {
			tokenHandler(w, r)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	c := NewClient("test-key", append([]Option{WithBaseURL(srv.URL)}, opts...)...)
	c.now = func() time.Time { return testNow }
	return c, rec
}

// serveJSON answers every request with status and body.
func serveJSON(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// serveFixture answers every request with the contents of testdata/<name>.
func serveFixture(t *testing.T, name string) http.HandlerFunc {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return serveJSON(http.StatusOK, string(body))
}
