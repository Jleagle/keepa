package keepa

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

type fakeLimiter struct {
	calls int
	err   error
}

func (f *fakeLimiter) Wait(ctx context.Context) error {
	f.calls++
	return f.err
}

// captureLogs returns a logger whose output can be inspected.
func captureLogs() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestNewClientDefaults(t *testing.T) {
	c := NewClient("key")
	if c.apiKey != "key" {
		t.Errorf("apiKey = %q", c.apiKey)
	}
	if c.baseURL != "https://api.keepa.com" {
		t.Errorf("baseURL = %q", c.baseURL)
	}
	if c.reserve != 20 {
		t.Errorf("reserve = %d, want 20", c.reserve)
	}
	if c.timeout != time.Minute {
		t.Errorf("timeout = %v, want 1m", c.timeout)
	}
	if c.httpClient == nil || c.logger == nil || c.now == nil {
		t.Error("httpClient, logger and now must have defaults")
	}
	if c.limiter != nil || c.onTokens != nil {
		t.Error("limiter and callback default to nil")
	}
	if c.Tokens().Known {
		t.Error("a new client must not claim to know the bucket")
	}
}

func TestNewClientOptions(t *testing.T) {
	hc := &http.Client{}
	logger, _ := captureLogs()
	lim := &fakeLimiter{}
	called := false
	c := NewClient("key",
		WithHTTPClient(hc),
		WithBaseURL("http://example.test/"),
		WithLimiter(lim),
		WithLogger(logger),
		WithTokenCallback(func(TokenUpdate) { called = true }),
		WithTokenReserve(1000),
		WithTimeout(5*time.Second),
	)
	if c.httpClient != hc {
		t.Error("WithHTTPClient not applied")
	}
	if c.baseURL != "http://example.test" {
		t.Errorf("WithBaseURL should trim the trailing slash: %q", c.baseURL)
	}
	if c.limiter != lim {
		t.Error("WithLimiter not applied")
	}
	if c.logger != logger {
		t.Error("WithLogger not applied")
	}
	if c.reserve != 1000 {
		t.Errorf("reserve = %d, want 1000", c.reserve)
	}
	if c.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", c.timeout)
	}
	c.onTokens(TokenUpdate{})
	if !called {
		t.Error("WithTokenCallback not applied")
	}
}
