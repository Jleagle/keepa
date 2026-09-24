package keepa

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL      = "https://api.keepa.com"
	defaultTimeout      = time.Minute
	defaultTokenReserve = 20
)

// Limiter paces requests before the token wait. *rate.Limiter from
// golang.org/x/time/rate satisfies it.
type Limiter interface {
	Wait(ctx context.Context) error
}

// Client calls the Keepa API. It is safe for concurrent use.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	limiter    Limiter
	logger     *slog.Logger
	onTokens   func(TokenUpdate)
	reserve    int
	timeout    time.Duration
	now        func() time.Time

	tokens tokenBucket
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets the transport. The default has no timeout of its own;
// every request carries a context deadline, see WithTimeout.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// WithBaseURL points the client at another server, for example a test server.
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

// WithLimiter waits on l before every request, ahead of the token wait.
func WithLimiter(l Limiter) Option { return func(c *Client) { c.limiter = l } }

// WithLogger receives the client's diagnostics. The default discards them.
func WithLogger(l *slog.Logger) Option { return func(c *Client) { c.logger = l } }

// WithTokenCallback is invoked for every Keepa envelope received, including
// error envelopes, with the token counts it carried. It runs synchronously on
// the goroutine that made the request, so it should return quickly.
func WithTokenCallback(fn func(TokenUpdate)) Option { return func(c *Client) { c.onTokens = fn } }

// WithTokenReserve sets the number of tokens the client keeps in hand. A call
// that would drop the bucket below it waits until the refill covers it.
// Individual calls override it with WithReserve. The default is 20.
func WithTokenReserve(n int) Option { return func(c *Client) { c.reserve = n } }

// WithTimeout sets the deadline applied to a request whose context has none.
// The default is one minute; best sellers uses double. Zero or a negative
// value disables the fallback, leaving requests bounded only by the caller's
// context and the HTTP client.
func WithTimeout(d time.Duration) Option { return func(c *Client) { c.timeout = d } }

// NewClient returns a client for the given API key.
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{},
		logger:     slog.New(slog.DiscardHandler),
		reserve:    defaultTokenReserve,
		timeout:    defaultTimeout,
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}
