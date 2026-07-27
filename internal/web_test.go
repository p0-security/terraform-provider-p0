// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package internal

import (
	"io"
	"net/http"
	"net/http/httptest"
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
