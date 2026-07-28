// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package internal

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDoBodyNilRequestSendsNoBody guards against sending the literal JSON `null`
// (which strict body parsers reject) for bodyless writes such as role bindings.
func TestDoBodyNilRequestSendsNoBody(t *testing.T) {
	var gotBody, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	data := P0ProviderData{BaseUrl: server.URL, Authentication: "Bearer x", Client: server.Client()}
	if _, err := data.Put("some/path", nil, nil); err != nil {
		t.Fatalf("Put with nil body returned error: %v", err)
	}
	if gotBody != "" {
		t.Errorf("expected empty request body, got %q", gotBody)
	}
	if gotContentType != "" {
		t.Errorf("expected no Content-Type header, got %q", gotContentType)
	}
}

// TestDoBodyTypedNilRequestSendsNoBody guards against the typed-nil variant of
// the same bug: a nil pointer boxed in the `any` parameter is != nil as an
// interface comparison, but should still be treated as "no body" rather than
// marshaled to the JSON literal `null`.
func TestDoBodyTypedNilRequestSendsNoBody(t *testing.T) {
	var gotBody, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	data := P0ProviderData{BaseUrl: server.URL, Authentication: "Bearer x", Client: server.Client()}
	type payload struct{ Foo string }
	var typedNil *payload
	if _, err := data.Put("some/path", typedNil, nil); err != nil {
		t.Fatalf("Put with typed-nil body returned error: %v", err)
	}
	if gotBody != "" {
		t.Errorf("expected empty request body, got %q", gotBody)
	}
	if gotContentType != "" {
		t.Errorf("expected no Content-Type header, got %q", gotContentType)
	}
}

// TestDeleteSurfacesBackendErrorMessage guards against swallowing the P0
// backend's actual error message (e.g. "Cannot remove the last owner", sent as
// {"error": "..."} with a 422) behind a generic "unexpected return code"
// message.
func TestDeleteSurfacesBackendErrorMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"Cannot remove the last owner"}`))
	}))
	defer server.Close()

	data := P0ProviderData{BaseUrl: server.URL, Authentication: "Bearer x", Client: server.Client()}
	_, err := data.Delete("settings/roles/owner/bindings/users/owner@example.com")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "Cannot remove the last owner") {
		t.Errorf("expected error to contain the backend message, got %q", err.Error())
	}
}

// TestDeleteFallsBackWhenBodyIsNotJsonError confirms Delete still reports a
// useful error when the error response body isn't the expected JSON shape.
func TestDeleteFallsBackWhenBodyIsNotJsonError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	data := P0ProviderData{BaseUrl: server.URL, Authentication: "Bearer x", Client: server.Client()}
	_, err := data.Delete("some/path")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected fallback error to mention the status code, got %q", err.Error())
	}
}

// TestUserAgentSentOnAllMethods confirms every request shape (GET, POST, and
// DELETE, which bypasses Do) carries the configured User-Agent so P0 can
// attribute API traffic to the Terraform provider.
func TestUserAgentSentOnAllMethods(t *testing.T) {
	var gotUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	const ua = "terraform-provider-p0/test Terraform/1.9.0"
	data := P0ProviderData{BaseUrl: server.URL, Authentication: "Bearer x", UserAgent: ua, Client: server.Client()}

	calls := map[string]func() error{
		"Get":    func() error { _, err := data.Get("some/path", nil); return err },
		"Post":   func() error { _, err := data.Post("some/path", nil, nil); return err },
		"Delete": func() error { _, err := data.Delete("some/path"); return err },
	}
	for name, call := range calls {
		gotUserAgent = ""
		if err := call(); err != nil {
			t.Fatalf("%s returned error: %v", name, err)
		}
		if gotUserAgent != ua {
			t.Errorf("%s: expected User-Agent %q, got %q", name, ua, gotUserAgent)
		}
	}
}

// TestDoBodyMarshalsRequest confirms non-nil requests are still marshaled as
// JSON with the appropriate Content-Type.
func TestDoBodyMarshalsRequest(t *testing.T) {
	var gotBody, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	data := P0ProviderData{BaseUrl: server.URL, Authentication: "Bearer x", Client: server.Client()}
	var resp map[string]any
	if _, err := data.Post("some/path", map[string]string{"unit": "d"}, &resp); err != nil {
		t.Fatalf("Post returned error: %v", err)
	}
	if gotBody != `{"unit":"d"}` {
		t.Errorf("expected JSON body, got %q", gotBody)
	}
	if gotContentType != "application/json" {
		t.Errorf("expected application/json Content-Type, got %q", gotContentType)
	}
}
