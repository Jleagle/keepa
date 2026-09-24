package keepa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Envelope is the metadata Keepa includes in every response, including
// error responses. Every response type embeds it.
type Envelope struct {
	Timestamp          int64     `json:"timestamp"`  // server time, Unix milliseconds
	TokensLeft         int       `json:"tokensLeft"` // may be negative
	TokensConsumed     int       `json:"tokensConsumed"`
	RefillIn           int       `json:"refillIn"`           // milliseconds until the next refill
	RefillRate         int       `json:"refillRate"`         // tokens per minute
	TokenFlowReduction float64   `json:"tokenFlowReduction"` // tokens per minute lost to tracking
	ProcessingTimeInMs int       `json:"processingTimeInMs"`
	Error              *APIError `json:"error"` // StatusCode is set by the client
}

// isEnvelope distinguishes a Keepa body from an unrelated JSON error page.
func (e *Envelope) isEnvelope() bool { return e.Timestamp > 0 || e.RefillRate > 0 }

// request describes one API call.
type request struct {
	path    string        // "/product"
	query   url.Values    // includes the API key
	body    any           // JSON-encoded and POSTed when non-nil
	cost    int           // expected token cost
	timeout time.Duration // fallback deadline; 0 means the client default
	callParams
}

// query returns query values with the API key set.
func (c *Client) query() url.Values {
	vals := url.Values{}
	vals.Set("key", c.apiKey)
	return vals
}

// do executes r and decodes the body into T.
func do[T any](ctx context.Context, c *Client, r request) (*T, error) {
	if r.cost > 0 {
		c.seed(ctx)
	}

	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}
	}

	reserve := c.reserve
	if r.reserve != nil {
		reserve = *r.reserve
	}
	if err := c.waitForTokens(ctx, r.cost, reserve, r.noWait); err != nil {
		return nil, err
	}

	if _, ok := ctx.Deadline(); !ok {
		timeout := r.timeout
		if timeout == 0 {
			timeout = c.timeout
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	req, err := c.newRequest(ctx, r)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// The envelope is read before the status check: an error response,
	// notably the 429 when the quota runs out, still carries the
	// authoritative token counts. An envelope without a refill rate (Keepa
	// sends zeros with parameter errors) carries no reading and is skipped.
	var env Envelope
	decodeErr := json.Unmarshal(body, &env)
	switch {
	case decodeErr == nil && env.isEnvelope():
		if env.RefillRate > 0 {
			c.recordEnvelope(&env, r.path)
			if env.TokensConsumed > r.cost {
				c.logger.Warn("keepa: token cost mismatch", "path", r.path, "expected", r.cost, "actual", env.TokensConsumed)
			}
		}
		if env.Error != nil {
			env.Error.StatusCode = resp.StatusCode
			return nil, env.Error
		}
	case decodeErr != nil:
		c.logger.Warn("keepa: undecodable response", "path", r.path, "status", resp.StatusCode, "body", truncate(body, 512))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: body}
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("keepa: decoding %s response: %w", r.path, decodeErr)
	}

	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("keepa: decoding %s response: %w", r.path, err)
	}
	return &out, nil
}

func (c *Client) newRequest(ctx context.Context, r request) (*http.Request, error) {
	u := c.baseURL + r.path + "?" + r.query.Encode()
	if r.body == nil {
		return http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	}
	b, err := json.Marshal(r.body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}
