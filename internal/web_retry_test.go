// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package internal

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// shortRetryDelays makes backoff without a Retry-After header take
// milliseconds, and the retry window 100ms, for the duration of a test.
func shortRetryDelays(t *testing.T) {
	t.Helper()
	oldBase, oldMax, oldWindow := baseRetryDelay, maxRetryDelay, retryWindow
	baseRetryDelay, maxRetryDelay, retryWindow = time.Millisecond, 10*time.Millisecond, 100*time.Millisecond
	t.Cleanup(func() { baseRetryDelay, maxRetryDelay, retryWindow = oldBase, oldMax, oldWindow })
}

// The page P0's load balancer sends with a 429.
const rateLimitPage = "<html><body><h1>429 Too Many Requests</h1></body></html>"

// TestRetriesRateLimitThenSucceeds confirms a request refused with a 429 is
// sent again, body included, and its eventual answer is returned.
func TestRetriesRateLimitThenSucceeds(t *testing.T) {
	shortRetryDelays(t)
	var calls atomic.Int32
	var lastBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		lastBody = string(b)
		if calls.Add(1) <= 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(rateLimitPage))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	data := P0ProviderData{BaseUrl: server.URL, Authentication: "Bearer x", Client: server.Client()}
	var resp map[string]any
	if _, err := data.Post("some/path", map[string]string{"unit": "d"}, &resp); err != nil {
		t.Fatalf("Post returned error: %v", err)
	}
	if calls.Load() != 4 {
		t.Errorf("expected 4 requests, got %d", calls.Load())
	}
	if lastBody != `{"unit":"d"}` {
		t.Errorf("expected the retried request to carry the body, got %q", lastBody)
	}
	if resp["ok"] != true {
		t.Errorf("expected the final response to be parsed, got %v", resp)
	}
}

// TestRateLimitExhaustedSaysSo guards against reporting P0's rate limit as
// "invalid character '<'": the load balancer's 429 is an HTML page, and a
// request that stays limited must fail with an error that names the limit.
func TestRateLimitExhaustedSaysSo(t *testing.T) {
	shortRetryDelays(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(rateLimitPage))
	}))
	defer server.Close()

	data := P0ProviderData{BaseUrl: server.URL, Authentication: "Bearer x", Client: server.Client()}
	methods := map[string]func() error{
		"Get":    func() error { _, err := data.Get("some/path", nil); return err },
		"Put":    func() error { _, err := data.Put("some/path", map[string]string{"a": "b"}, nil); return err },
		"Delete": func() error { _, err := data.Delete("some/path"); return err },
	}
	for name, call := range methods {
		calls.Store(0)
		start := time.Now()
		err := call()
		elapsed := time.Since(start)
		if err == nil {
			t.Fatalf("%s: expected an error, got nil", name)
		}
		if strings.Contains(err.Error(), "invalid character") {
			t.Errorf("%s: expected a rate limit error, got a JSON parse error: %q", name, err.Error())
		}
		if !strings.Contains(err.Error(), "rate limiting") || !strings.Contains(err.Error(), "429") {
			t.Errorf("%s: expected the error to name the rate limit, got %q", name, err.Error())
		}
		if got := calls.Load(); got < 2 {
			t.Errorf("%s: expected the request to be retried, got %d requests", name, got)
		}
		if elapsed < retryWindow || elapsed > retryWindow+time.Second {
			t.Errorf("%s: expected to give up once the %s retry window passed, gave up after %s", name, retryWindow, elapsed)
		}
	}
}

// TestNonJsonErrorReportsStatus confirms an error page that isn't JSON (here a
// 502 from a proxy) is reported by its status, not as a JSON parse error.
func TestNonJsonErrorReportsStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>Bad Gateway</html>"))
	}))
	defer server.Close()

	data := P0ProviderData{BaseUrl: server.URL, Authentication: "Bearer x", Client: server.Client()}
	_, err := data.Get("some/path", nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "502") || strings.Contains(err.Error(), "invalid character") {
		t.Errorf("expected the error to report the status, got %q", err.Error())
	}
}

// TestRetryAfterIsHonored confirms the provider waits as long as P0's
// Retry-After header asks before sending the request again.
func TestRetryAfterIsHonored(t *testing.T) {
	var mu sync.Mutex
	var times []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		first := len(times) == 1
		mu.Unlock()
		if first {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	data := P0ProviderData{BaseUrl: server.URL, Authentication: "Bearer x", Client: server.Client()}
	if _, err := data.Get("some/path", nil); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if len(times) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(times))
	}
	if waited := times[1].Sub(times[0]); waited < 900*time.Millisecond {
		t.Errorf("expected to wait about 1s for Retry-After, waited %s", waited)
	}
}

// TestThrottleSharedDuringConcurrentRequests confirms that while one request
// is waiting out a 429, a concurrent request isn't sent either.
func TestThrottleSharedDuringConcurrentRequests(t *testing.T) {
	var mu sync.Mutex
	var limitedAt, otherAt time.Time
	limited := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/limited" && limitedAt.IsZero() {
			limitedAt = time.Now()
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			close(limited)
			return
		}
		if r.URL.Path == "/other" {
			otherAt = time.Now()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	data := P0ProviderData{BaseUrl: server.URL, Authentication: "Bearer x", Client: server.Client(), Throttle: NewThrottle()}
	other := data

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := data.Get("limited", nil); err != nil {
			t.Errorf("Get returned error: %v", err)
		}
	}()
	<-limited
	// Give the limited request a moment to record its pause.
	time.Sleep(50 * time.Millisecond)
	if _, err := other.Get("other", nil); err != nil {
		t.Fatalf("Get through the copy returned error: %v", err)
	}
	wg.Wait()
	if waited := otherAt.Sub(limitedAt); waited < 900*time.Millisecond {
		t.Errorf("expected the concurrent request to wait for the 429's Retry-After, it was sent after %s", waited)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 15, 22, 10, 0, 0, time.UTC)
	cases := []struct {
		header string
		want   time.Duration
		ok     bool
	}{
		{"", 0, false},
		{"5", 5 * time.Second, true},
		{"0", 0, true},
		{"-1", 0, false},
		{"soon", 0, false},
		{"3600", maxRetryDelay, true},
		{now.Add(10 * time.Second).Format(http.TimeFormat), 10 * time.Second, true},
		{now.Add(-10 * time.Second).Format(http.TimeFormat), 0, true},
	}
	for _, c := range cases {
		got, ok := parseRetryAfter(c.header, now)
		if got != c.want || ok != c.ok {
			t.Errorf("parseRetryAfter(%q) = %s, %v; want %s, %v", c.header, got, ok, c.want, c.ok)
		}
	}
}

// TestRetryDelayStaysCapped guards computeRetryDelay over the many attempts a
// five-minute window allows, including attempts large enough to overflow the
// doubling.
func TestRetryDelayStaysCapped(t *testing.T) {
	for attempt := 0; attempt < 100; attempt++ {
		if d := computeRetryDelay(attempt); d < baseRetryDelay || d > maxRetryDelay+maxRetryDelay/4 {
			t.Errorf("attempt %d: delay %s is outside [%s, %s]", attempt, d, baseRetryDelay, maxRetryDelay+maxRetryDelay/4)
		}
	}
}

// TestRetryAfterZeroStillBacksOff confirms a Retry-After of 0 doesn't send
// requests back to back for the whole retry window.
func TestRetryAfterZeroStillBacksOff(t *testing.T) {
	shortRetryDelays(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	data := P0ProviderData{BaseUrl: server.URL, Authentication: "Bearer x", Client: server.Client()}
	if _, err := data.Get("some/path", nil); err == nil {
		t.Fatal("expected an error, got nil")
	}
	// At most one request per baseRetryDelay, plus the first.
	if limit := int32(retryWindow/baseRetryDelay) + 1; calls.Load() > limit {
		t.Errorf("expected at most %d requests in the %s window, got %d", limit, retryWindow, calls.Load())
	}
}
