// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/p0-security/terraform-provider-p0/internal"
)

const (
	stubOrg   = "test-org"
	stubToken = "test-token"
)

// An integration's install spec, as the stub enforces it.
type stubInstallSpec struct {
	integration string
	components  map[string]stubComponent
}

// One component of an install spec.
type stubComponent struct {
	// Every element of the component's schema. The stub drops any other field,
	// as P0 does, so a field the provider misnames reads back empty.
	fields []string
	// The elements marked `step: "new"`, which only staging may change.
	stepNew []string
}

// installApiStub serves P0's install API for one integration, closely enough to
// drive the provider's install resources without a P0 organization. It follows
// install-api.ts and configure.ts in the app: PUT stages an item, verify and
// configure advance it, and a `step: "new"` field can't change after staging.
// It runs no install verifiers, so it can't tell whether the AWS or Databricks
// side of an install exists.
type installApiStub struct {
	stubInstallSpec

	mu        sync.Mutex
	installed bool
	// Items by component, then by item ID.
	items map[string]map[string]map[string]any
}

// newInstallApiStub starts a stub of the integration's install API, and returns
// a provider block and an API client that point at it.
func newInstallApiStub(t *testing.T, spec stubInstallSpec) (string, *internal.P0ProviderData) {
	stub := &installApiStub{
		stubInstallSpec: spec,
		items:           map[string]map[string]map[string]any{},
	}
	server := httptest.NewServer(stub)
	t.Cleanup(server.Close)

	provider := fmt.Sprintf(`
provider "p0" {
  org       = %q
  host      = %q
  api_token = %q
}
`, stubOrg, server.URL, stubToken)
	client := &internal.P0ProviderData{
		BaseUrl:        fmt.Sprintf("%s/o/%s", server.URL, stubOrg),
		Authentication: "Bearer " + stubToken,
		Client:         server.Client(),
	}
	return provider, client
}

func (s *installApiStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+stubToken {
		writeStubError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	rest, ok := strings.CutPrefix(r.URL.Path, fmt.Sprintf("/o/%s/integrations/%s/config", stubOrg, s.integration))
	if !ok {
		writeStubError(w, http.StatusNotFound, "Not found")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if rest == "" && r.Method == http.MethodPost {
		if s.installed {
			writeStubError(w, http.StatusConflict, "Integration is already installed")
			return
		}
		s.installed = true
		writeStubJson(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	parts := strings.Split(strings.TrimPrefix(rest, "/"), "/")
	if len(parts) < 2 || len(parts) > 3 {
		writeStubError(w, http.StatusNotFound, "Not found")
		return
	}
	component, id := parts[0], parts[1]
	spec, ok := s.components[component]
	if !ok {
		writeStubError(w, http.StatusNotFound, "Not found")
		return
	}
	if !s.installed {
		writeStubError(w, http.StatusBadRequest, "Integration is not installed")
		return
	}
	if s.items[component] == nil {
		s.items[component] = map[string]map[string]any{}
	}
	item, exists := s.items[component][id]

	switch {
	case len(parts) == 2 && r.Method == http.MethodGet:
		if !exists {
			writeStubError(w, http.StatusNotFound, id)
			return
		}
		writeStubJson(w, http.StatusOK, map[string]any{"ok": true, "item": item})
	case len(parts) == 2 && r.Method == http.MethodPut:
		s.update(w, r, spec, component, id, "assemble")
	case len(parts) == 2 && r.Method == http.MethodDelete:
		delete(s.items[component], id)
		w.WriteHeader(http.StatusNoContent)
	case len(parts) == 3 && r.Method == http.MethodPost && (parts[2] == "verify" || parts[2] == "configure"):
		if !exists {
			writeStubError(w, http.StatusNotFound, id)
			return
		}
		s.update(w, r, spec, component, id, parts[2])
	default:
		writeStubError(w, http.StatusNotFound, "Not found")
	}
}

// update runs one install step on an item: merges the request into it, keeps
// only the component's fields, and moves it to the step's next state.
func (s *installApiStub) update(w http.ResponseWriter, r *http.Request, spec stubComponent, component, id, step string) {
	previous := s.items[component][id]
	if step == "configure" && previous["state"] != "configure" && previous["state"] != "installed" {
		writeStubError(w, http.StatusBadRequest, "Invalid integration state; must be one of configure, installed")
		return
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeStubError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %s", err))
		return
	}

	updated := map[string]any{}
	for _, field := range spec.fields {
		value, sent := body[field]
		if !sent || value == nil {
			value, sent = previous[field]
		}
		if !sent {
			continue
		}
		if step != "assemble" && slices.Contains(spec.stepNew, field) && !reflect.DeepEqual(value, previous[field]) {
			writeStubError(w, http.StatusBadRequest, fmt.Sprintf("'%s' can only be altered on initial installation. Create a new installation to change this field.", field))
			return
		}
		updated[field] = value
	}
	updated["state"] = map[string]string{"assemble": "stage", "verify": "configure", "configure": "installed"}[step]

	s.items[component][id] = updated
	writeStubJson(w, http.StatusOK, map[string]any{"ok": true, "item": updated})
}

func writeStubJson(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeStubError(w http.ResponseWriter, status int, message string) {
	writeStubJson(w, status, map[string]any{"error": message})
}
