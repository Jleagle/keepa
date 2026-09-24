package keepa

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAPIErrorIsNotEnoughTokens(t *testing.T) {
	err := error(&APIError{StatusCode: http.StatusTooManyRequests, Type: "notEnoughTokens", Message: "no tokens"})
	if !errors.Is(err, ErrNotEnoughTokens) {
		t.Error("429 APIError should match ErrNotEnoughTokens")
	}
	if errors.Is(&APIError{StatusCode: 400, Type: "invalidParameter"}, ErrNotEnoughTokens) {
		t.Error("400 APIError should not match ErrNotEnoughTokens")
	}
	if errors.Is(err, ErrInvalidRequest) {
		t.Error("APIError should not match ErrInvalidRequest")
	}
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok || apiErr.Type != "notEnoughTokens" {
		t.Errorf("errors.AsType failed: %v %v", apiErr, ok)
	}
}

func TestAPIErrorString(t *testing.T) {
	err := &APIError{StatusCode: 400, Type: "invalidParameter", Message: "bad key", Details: "see docs"}
	got := err.Error()
	for _, want := range []string{"keepa:", "invalidParameter", "400", "bad key", "see docs"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, missing %q", got, want)
		}
	}
	plain := (&APIError{StatusCode: 400, Type: "x", Message: "m"}).Error()
	if strings.Contains(plain, "()") {
		t.Errorf("empty details should not print parentheses: %q", plain)
	}
}

func TestHTTPErrorString(t *testing.T) {
	err := &HTTPError{StatusCode: 502, Body: []byte("gateway blew up")}
	got := err.Error()
	if !strings.HasPrefix(got, "keepa: HTTP 502") || !strings.Contains(got, "gateway blew up") {
		t.Errorf("Error() = %q", got)
	}
	long := &HTTPError{StatusCode: 500, Body: []byte(strings.Repeat("x", 500))}
	if len(long.Error()) > 260 {
		t.Errorf("long body not truncated: %d chars", len(long.Error()))
	}
}

func TestTokenWaitErrorIsWouldWait(t *testing.T) {
	err := error(&TokenWaitError{Wait: 90 * time.Second, Cost: 50, Reserve: 1000, Projected: 980})
	if !errors.Is(err, ErrWouldWait) {
		t.Error("TokenWaitError should match ErrWouldWait")
	}
	got := err.Error()
	for _, want := range []string{"keepa:", "50", "1000", "1m30s"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, missing %q", got, want)
		}
	}
}

func TestInvalidRequest(t *testing.T) {
	err := invalidRequest("between 1 and %d ASINs required, got %d", 100, 0)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Error("invalidRequest should wrap ErrInvalidRequest")
	}
	if got := err.Error(); got != "keepa: invalid request: between 1 and 100 ASINs required, got 0" {
		t.Errorf("Error() = %q", got)
	}
}
