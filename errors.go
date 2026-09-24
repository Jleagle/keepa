package keepa

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

var (
	// ErrInvalidRequest wraps every argument validation failure.
	ErrInvalidRequest = errors.New("keepa: invalid request")
	// ErrNotEnoughTokens matches an *APIError carried by an HTTP 429 response.
	ErrNotEnoughTokens = errors.New("keepa: not enough tokens")
	// ErrWouldWait matches a *TokenWaitError from a call made WithoutWaiting.
	ErrWouldWait = errors.New("keepa: call would wait for tokens")
)

// APIError is the error object Keepa includes in a response envelope.
type APIError struct {
	StatusCode int    `json:"-"` // HTTP status of the response that carried it
	Type       string `json:"type"`
	Message    string `json:"message"`
	Details    string `json:"details"`
}

func (e *APIError) Error() string {
	s := fmt.Sprintf("keepa: api error %s (HTTP %d): %s", e.Type, e.StatusCode, e.Message)
	if e.Details != "" {
		s += " (" + e.Details + ")"
	}
	return s
}

// Is reports ErrNotEnoughTokens for a 429 response.
func (e *APIError) Is(target error) bool {
	return target == ErrNotEnoughTokens && e.StatusCode == http.StatusTooManyRequests
}

// HTTPError is a non-200 response whose body was not a Keepa envelope.
type HTTPError struct {
	StatusCode int
	Body       []byte
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("keepa: HTTP %d: %s", e.StatusCode, truncate(e.Body, 200))
}

// TokenWaitError is returned by a call made WithoutWaiting when the bucket
// cannot pay for it without dropping below the reserve. Wait is how long the
// client would have slept.
type TokenWaitError struct {
	Wait      time.Duration
	Cost      int
	Reserve   int
	Projected float64
}

func (e *TokenWaitError) Error() string {
	return fmt.Sprintf("keepa: call costing %d tokens would wait %s to keep %d in reserve (%.0f projected)",
		e.Cost, e.Wait.Round(time.Second), e.Reserve, e.Projected)
}

// Is reports ErrWouldWait.
func (e *TokenWaitError) Is(target error) bool { return target == ErrWouldWait }

func invalidRequest(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRequest, fmt.Sprintf(format, args...))
}

// truncate returns b as a string, cut to n bytes with an ellipsis.
func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}
