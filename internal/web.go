// Copyright (c) 2024 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"time"
)

// Like http.Client, except:
//   - handles authentication
//   - converts to/from JSON
//   - treats response codes 400 and higher as errors
//   - retries 429 responses with exponential backoff, honoring Retry-After
type P0ProviderData struct {
	BaseUrl        string
	Authentication string
	UserAgent      string
	Client         *http.Client
	// Throttle, when set, is shared by every copy of this P0ProviderData, so
	// that a 429 on one request pauses all of the provider's requests rather
	// than only the one that was refused. Terraform runs up to 10 operations
	// at once, and P0 limits requests per IP address, so retrying only the
	// refused request lets the others keep the limit exhausted.
	Throttle *Throttle
}

// Throttle holds the time until which requests to P0 should wait after a 429.
type Throttle struct {
	mu    sync.Mutex
	until time.Time
}

func NewThrottle() *Throttle {
	return &Throttle{}
}

// pauseUntil extends the pause to t. It never shortens a pause.
func (t *Throttle) pauseUntil(until time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if until.After(t.until) {
		t.until = until
	}
}

// wait blocks until any pause has passed, or done is closed.
func (t *Throttle) wait(done <-chan struct{}) bool {
	if t == nil {
		return true
	}
	for {
		t.mu.Lock()
		remaining := time.Until(t.until)
		t.mu.Unlock()
		if remaining <= 0 {
			return true
		}
		select {
		case <-time.After(remaining):
		case <-done:
			return false
		}
	}
}

// Variables rather than constants so tests can shorten them.
var (
	baseRetryDelay = 1 * time.Second
	maxRetryDelay  = 60 * time.Second
	// How long a request keeps retrying 429s, counted from its first attempt.
	// P0 counts requests per minute, so this spans several windows.
	retryWindow = 5 * time.Minute
)

// doWithRetry sends req via the underlying http.Client, retrying on 429
// responses. It waits for the Retry-After header when P0 sends one, and
// otherwise backs off exponentially with jitter.
func (data *P0ProviderData) doWithRetry(req *http.Request) (*http.Response, error) {
	if data.UserAgent != "" {
		req.Header.Set("User-Agent", data.UserAgent)
	}

	deadline := time.Now().Add(retryWindow)

	for attempt := 0; ; attempt++ {
		if attempt > 0 && req.GetBody != nil {
			newBody, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			req.Body = newBody
		}

		if !data.Throttle.wait(req.Context().Done()) {
			return nil, req.Context().Err()
		}

		resp, err := data.Client.Do(req)
		if err != nil {
			return resp, err
		}

		remaining := time.Until(deadline)
		if resp.StatusCode != http.StatusTooManyRequests || remaining <= 0 {
			return resp, nil
		}

		delay, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		if !ok {
			delay = computeRetryDelay(attempt)
		}
		// Wait at least the base delay, so a Retry-After of 0 can't send
		// requests back to back for the whole window, and make the last
		// attempt at the deadline rather than giving up early.
		delay = min(max(delay, baseRetryDelay), remaining)
		data.Throttle.pauseUntil(time.Now().Add(delay))

		// Drain and close so the underlying connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		select {
		case <-time.After(delay):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
}

// computeRetryDelay returns an exponential backoff delay with jitter,
// capped at maxRetryDelay.
func computeRetryDelay(attempt int) time.Duration {
	backoff := baseRetryDelay * (1 << attempt)
	if backoff > maxRetryDelay || backoff <= 0 {
		backoff = maxRetryDelay
	}
	jitter := time.Duration(0)
	if quarter := int64(backoff / 4); quarter > 0 {
		jitter = time.Duration(rand.Int63n(quarter))
	}
	return backoff + jitter
}

// parseRetryAfter reads a Retry-After header, given either as seconds or as
// an HTTP date, and caps it at maxRetryDelay. It reports false when the
// header is missing or malformed.
func parseRetryAfter(header string, now time.Time) (time.Duration, bool) {
	if header == "" {
		return 0, false
	}
	var delay time.Duration
	if seconds, err := strconv.Atoi(header); err == nil {
		if seconds < 0 {
			return 0, false
		}
		delay = time.Duration(seconds) * time.Second
	} else if date, err := http.ParseTime(header); err == nil {
		delay = date.Sub(now)
		if delay < 0 {
			delay = 0
		}
	} else {
		return 0, false
	}
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}
	return delay, true
}

// statusError describes an error response whose body isn't P0's JSON, such
// as the HTML page P0's load balancer sends with a 429.
func statusError(resp *http.Response) error {
	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf(
			"P0 is still rate limiting this provider's requests (%s) after retrying for %g minutes. "+
				"Wait a minute and run Terraform again with fewer concurrent operations, for example -parallelism=2",
			resp.Status, retryWindow.Minutes(),
		)
	}
	return fmt.Errorf("unexpected response from P0: %s", resp.Status)
}

func (data *P0ProviderData) Do(req *http.Request, responseJson any) (*http.Response, error) {
	req.Header.Add("Accept", "application/json")

	resp, errDo := data.doWithRetry(req)
	if errDo != nil {
		return resp, errDo
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return resp, readErr
	}

	// Some endpoints acknowledge success with an empty body (e.g. 201/204 from
	// role-binding writes). There is no JSON to parse, so treat the status code
	// as authoritative and leave responseJson at its zero value.
	if len(bytes.TrimSpace(body)) == 0 {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}
		return resp, statusError(resp)
	}

	// An error response that isn't JSON (e.g. a load balancer's HTML page)
	// would otherwise surface as a JSON parse error such as
	// "invalid character '<'".
	if resp.StatusCode >= 400 && !json.Valid(body) {
		return resp, statusError(resp)
	}

	parseErr := json.Unmarshal(body, &responseJson)
	if parseErr != nil {
		return resp, parseErr
	}

	// If the response contains "error", throw that here
	var generic map[string]any
	genericErr := json.Unmarshal(body, &generic)
	if genericErr != nil {
		return resp, genericErr
	}
	if generic["error"] != nil || resp.StatusCode >= 400 {
		errorText, ok := generic["error"].(string)
		if !ok {
			errorErr := fmt.Errorf(
				"got error response from P0, but error was not a string. Please contact support@p0.dev. HTTP status code: %s",
				resp.Status,
			)
			return resp, errorErr
		}
		return resp, fmt.Errorf("%s: %s", resp.Status, errorText)
	}

	return resp, nil
}

func (data *P0ProviderData) Get(path string, responseJson any) (*http.Response, error) {
	req, errNew := http.NewRequest("GET", fmt.Sprintf("%s/%s", data.BaseUrl, path), nil)
	if errNew != nil {
		return nil, errNew
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", data.Authentication)
	return data.Do(req, responseJson)
}

func (data *P0ProviderData) Delete(path string) (*http.Response, error) {
	req, errNew := http.NewRequest("DELETE", fmt.Sprintf("%s/%s", data.BaseUrl, path), nil)
	if errNew != nil {
		return nil, errNew
	}
	req.Header.Add("Authorization", data.Authentication)

	resp, errDo := data.doWithRetry(req)
	if errDo != nil {
		return resp, errDo
	}
	defer func() { _ = resp.Body.Close() }()

	// Endpoints should not return content on success.
	if resp.StatusCode == 204 {
		return resp, nil
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return resp, statusError(resp)
	}

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return resp, readErr
	}

	// Surface the P0 backend's actual error message (e.g. "Cannot remove the
	// last owner") instead of just the status code.
	var generic map[string]any
	if json.Unmarshal(body, &generic) == nil {
		if errorText, ok := generic["error"].(string); ok {
			return resp, fmt.Errorf("%s: %s", resp.Status, errorText)
		}
	}
	return resp, fmt.Errorf("unexpected return code during delete: %d", resp.StatusCode)
}

// isNilRequestBody reports whether requestJson represents "no body": either the
// untyped nil interface, or a typed nil (a nil pointer/map/slice/interface/
// func/chan boxed in the `any` parameter, which is != nil as an interface
// comparison but still marshals to the JSON literal `null`).
func isNilRequestBody(requestJson any) bool {
	if requestJson == nil {
		return true
	}
	v := reflect.ValueOf(requestJson)
	switch v.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func, reflect.Chan:
		return v.IsNil()
	default:
		return false
	}
}

func (data *P0ProviderData) doBody(method string, path string, requestJson any, responseJson any) (*http.Response, error) {
	// A nil requestJson (including a typed nil) means "no request body". Send an
	// empty body rather than the literal JSON `null`, which strict body parsers
	// (e.g. Express's express.json()) reject with a parse error.
	var reader io.Reader
	hasBody := !isNilRequestBody(requestJson)
	if hasBody {
		buf, marshalErr := json.Marshal(requestJson)
		if marshalErr != nil {
			return nil, marshalErr
		}
		reader = bytes.NewReader(buf)
	}

	req, errNew := http.NewRequest(method, fmt.Sprintf("%s/%s", data.BaseUrl, path), reader)
	if errNew != nil {
		return nil, errNew
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", data.Authentication)
	if hasBody {
		req.Header.Add("Content-Type", "application/json")
	}
	return data.Do(req, responseJson)
}

func (data *P0ProviderData) Post(path string, requestJson any, responseJson any) (*http.Response, error) {
	return data.doBody("POST", path, requestJson, responseJson)
}

func (data *P0ProviderData) Put(path string, requestJson any, responseJson any) (*http.Response, error) {
	return data.doBody("PUT", path, requestJson, responseJson)
}
