package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

const fakeOrg = "test-org"

// An in-memory stand-in for P0's generic install API, covering the behavior the
// install resources depend on:
//   - POST .../config creates the integration, and answers 409 once it exists
//   - PUT, verify and configure deep-merge the request body over the stored item, and
//     advance its state (stage -> configure -> installed)
//   - every item response carries the component's metadata, when it has any
//   - unknown items are 404, and DELETE answers 204 with no body
type fakeP0 struct {
	t      *testing.T
	server *httptest.Server

	mu sync.Mutex
	// integration -> component -> item id -> item
	integrations map[string]map[string]map[string]map[string]any
	// Called after each write, so a test can add fields the way P0's installers do.
	normalize func(integration, component, id string, item map[string]any)
	// Computes a component's metadata; returning nil omits the key, as P0 does.
	metadata func(integration, component, id string, item map[string]any) map[string]any
}

func newFakeP0(t *testing.T) *fakeP0 {
	f := &fakeP0{
		t:            t,
		integrations: map[string]map[string]map[string]map[string]any{},
		normalize:    func(string, string, string, map[string]any) {},
		metadata:     func(string, string, string, map[string]any) map[string]any { return nil },
	}

	mux := http.NewServeMux()
	base := "/o/" + fakeOrg + "/integrations/{integration}/config"
	item := base + "/{component}/{id}"
	mux.HandleFunc("POST "+base, f.postConfig)
	mux.HandleFunc("PUT "+item, f.step("stage", nil))
	mux.HandleFunc("POST "+item+"/verify", f.step("configure", nil))
	mux.HandleFunc("POST "+item+"/configure", f.step("installed", []string{"configure", "installed"}))
	mux.HandleFunc("GET "+item, f.getItem)
	mux.HandleFunc("DELETE "+item, f.deleteItem)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to the fake P0 API: %s %s", r.Method, r.URL.Path)
		notFound(w)
	})

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeP0) postConfig(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	integration := r.PathValue("integration")
	if _, ok := f.integrations[integration]; ok {
		http.Error(w, `{"error":"Integration is already installed"}`, http.StatusConflict)
		return
	}
	f.integrations[integration] = map[string]map[string]map[string]any{}
	writeJson(w, map[string]any{"ok": true, "config": map[string]any{}})
}

func (f *fakeP0) step(nextState string, expectedStates []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body == nil {
			http.Error(w, `{"error":"request body must be a JSON object"}`, http.StatusBadRequest)
			return
		}

		integration, component, id := r.PathValue("integration"), r.PathValue("component"), r.PathValue("id")
		components, ok := f.integrations[integration]
		if !ok {
			notFound(w)
			return
		}
		previous, exists := components[component][id]
		// Only staging may create an item.
		if !exists && nextState != "stage" {
			notFound(w)
			return
		}
		if expectedStates != nil && !contains(expectedStates, previous["state"]) {
			http.Error(w, `{"error":"Invalid integration state"}`, http.StatusBadRequest)
			return
		}

		updated := deepMerge(previous, body)
		// P0 never lets a request change an item's label.
		if label, ok := previous["label"]; ok {
			updated["label"] = label
		} else {
			updated["label"] = id
		}
		updated["state"] = nextState
		f.normalize(integration, component, id, updated)

		if components[component] == nil {
			components[component] = map[string]map[string]any{}
		}
		components[component][id] = updated
		f.writeItem(w, integration, component, id, updated)
	}
}

func (f *fakeP0) getItem(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	integration, component, id := r.PathValue("integration"), r.PathValue("component"), r.PathValue("id")
	stored, ok := f.integrations[integration][component][id]
	if !ok {
		notFound(w)
		return
	}
	f.writeItem(w, integration, component, id, stored)
}

func (f *fakeP0) deleteItem(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	integration, component, id := r.PathValue("integration"), r.PathValue("component"), r.PathValue("id")
	if _, ok := f.integrations[integration][component][id]; !ok {
		notFound(w)
		return
	}
	delete(f.integrations[integration][component], id)
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeP0) writeItem(w http.ResponseWriter, integration, component, id string, item map[string]any) {
	response := map[string]any{"ok": true, "item": item}
	if metadata := f.metadata(integration, component, id, item); metadata != nil {
		response["metadata"] = metadata
	}
	writeJson(w, response)
}

// Returns a copy of the stored item, or nil if it does not exist.
func (f *fakeP0) item(integration, component, id string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()

	stored, ok := f.integrations[integration][component][id]
	if !ok {
		return nil
	}
	return deepMerge(nil, stored)
}

// Changes a stored item behind Terraform's back, to simulate drift.
func (f *fakeP0) update(integration, component, id string, change func(item map[string]any)) {
	f.mu.Lock()
	defer f.mu.Unlock()

	change(f.integrations[integration][component][id])
}

func notFound(w http.ResponseWriter) {
	http.Error(w, `{"error":"Not found"}`, http.StatusNotFound)
}

func writeJson(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

// Merges overlay onto a copy of base, recursing into objects, like lodash's merge.
func deepMerge(base, overlay map[string]any) map[string]any {
	merged := map[string]any{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overlay {
		overlayObject, overlayIsObject := value.(map[string]any)
		baseObject, baseIsObject := merged[key].(map[string]any)
		switch {
		case overlayIsObject && baseIsObject:
			merged[key] = deepMerge(baseObject, overlayObject)
		case overlayIsObject:
			merged[key] = deepMerge(nil, overlayObject)
		default:
			merged[key] = value
		}
	}
	return merged
}

func contains(values []string, value any) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
