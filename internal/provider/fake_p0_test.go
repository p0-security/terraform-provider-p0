package provider

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
)

const fakeOrg = "test-org"

type fakeItemKey struct {
	integration, component, id string
}

// An in-memory stand-in for P0's generic install API, covering the behavior the
// install resources depend on:
//   - POST .../config creates the integration, and answers 409 once it exists
//   - PUT, verify and configure deep-merge the request body over the stored item, and
//     advance its state (stage -> configure -> installed)
//   - every item response carries the component's metadata, when it has any
//   - unknown items are 404, and DELETE answers 204 with no body
//   - a step that a test's check or refuse hook rejects answers 422 and saves nothing
//   - any other request fails the test
type fakeP0 struct {
	server *httptest.Server

	mu           sync.Mutex
	integrations map[string]bool
	items        map[fakeItemKey]map[string]any
	// Called after each write, so a test can add fields the way P0's installers do.
	normalize func(key fakeItemKey, item map[string]any)
	// Computes a component's metadata; returning nil omits the key, as P0 does.
	metadata func(key fakeItemKey, item map[string]any) map[string]any
	// Called before each write with the stored item (nil when staging creates it) and
	// the item the write would store. An error rejects the write, as P0 rejects a change
	// that its install schema doesn't allow, such as one to a `step: "new"` field after
	// staging.
	check func(key fakeItemKey, nextState string, previous, updated map[string]any) error
	// Whether the step that moves the item to nextState fails its install check, as when
	// a connector can't reach what the item names. P0 then saves nothing.
	refuse func(key fakeItemKey, nextState string) bool
}

func newFakeP0(t *testing.T) *fakeP0 {
	f := &fakeP0{
		integrations: map[string]bool{},
		items:        map[fakeItemKey]map[string]any{},
		normalize:    func(fakeItemKey, map[string]any) {},
		metadata:     func(fakeItemKey, map[string]any) map[string]any { return nil },
		check:        func(fakeItemKey, string, map[string]any, map[string]any) error { return nil },
		refuse:       func(fakeItemKey, string) bool { return false },
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

func itemKeyOf(r *http.Request) fakeItemKey {
	return fakeItemKey{r.PathValue("integration"), r.PathValue("component"), r.PathValue("id")}
}

func (f *fakeP0) postConfig(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	integration := r.PathValue("integration")
	if f.integrations[integration] {
		http.Error(w, `{"error":"Integration is already installed"}`, http.StatusConflict)
		return
	}
	f.integrations[integration] = true
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

		key := itemKeyOf(r)
		if !f.integrations[key.integration] {
			notFound(w)
			return
		}
		previous, exists := f.items[key]
		// Only staging may create an item.
		if !exists && nextState != "stage" {
			notFound(w)
			return
		}
		state, _ := previous["state"].(string)
		if expectedStates != nil && !slices.Contains(expectedStates, state) {
			http.Error(w, `{"error":"Invalid integration state"}`, http.StatusBadRequest)
			return
		}

		updated := deepMerge(previous, body)
		// P0 never lets a request change an item's label.
		if label, ok := previous["label"]; ok {
			updated["label"] = label
		} else {
			updated["label"] = key.id
		}
		updated["state"] = nextState
		f.normalize(key, updated)
		// P0 answers both with a StateError, a 422, before it saves the item.
		if err := f.check(key, nextState, previous, updated); err != nil {
			unprocessable(w, err.Error())
			return
		}
		if f.refuse(key, nextState) {
			unprocessable(w, "The install check failed")
			return
		}

		f.items[key] = updated
		f.writeItem(w, key, updated)
	}
}

func (f *fakeP0) getItem(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := itemKeyOf(r)
	stored, ok := f.items[key]
	if !ok {
		notFound(w)
		return
	}
	f.writeItem(w, key, stored)
}

func (f *fakeP0) deleteItem(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := itemKeyOf(r)
	if _, ok := f.items[key]; !ok {
		notFound(w)
		return
	}
	delete(f.items, key)
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeP0) writeItem(w http.ResponseWriter, key fakeItemKey, item map[string]any) {
	response := map[string]any{"ok": true, "item": item}
	if metadata := f.metadata(key, item); metadata != nil {
		response["metadata"] = metadata
	}
	writeJson(w, response)
}

// Returns a copy of the stored item, or nil if it does not exist.
func (f *fakeP0) item(integration, component, id string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()

	stored, ok := f.items[fakeItemKey{integration, component, id}]
	if !ok {
		return nil
	}
	return deepMerge(nil, stored)
}

// Changes a stored item behind Terraform's back, to simulate drift.
func (f *fakeP0) update(integration, component, id string, change func(item map[string]any)) {
	f.mu.Lock()
	defer f.mu.Unlock()

	change(f.items[fakeItemKey{integration, component, id}])
}

func notFound(w http.ResponseWriter) {
	http.Error(w, `{"error":"Not found"}`, http.StatusNotFound)
}

func unprocessable(w http.ResponseWriter, message string) {
	body, _ := json.Marshal(map[string]string{"error": message})
	http.Error(w, string(body), http.StatusUnprocessableEntity)
}

func writeJson(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

// Merges overlay onto a copy of base, recursing into objects, like lodash's merge.
func deepMerge(base, overlay map[string]any) map[string]any {
	merged := map[string]any{}
	maps.Copy(merged, base)
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
