// Copyright (c) 2026 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/p0-security/terraform-provider-p0/internal"
)

// Each Databricks component's install schema, as the app defines it in
// packages/integrations/databricks/shared/src/components.ts: its fields, and
// those marked `step: "new"`, which P0 refuses to change after staging.
var databricksSchema = map[string]struct{ fields, stepNew []string }{
	"connector": {fields: []string{"region", "domainPattern"}, stepNew: []string{"region", "domainPattern"}},
	"account":   {fields: []string{"connector", "accountsUrl", "applicationId"}, stepNew: []string{"connector", "accountsUrl"}},
	"workspace": {fields: []string{"account"}, stepNew: []string{"account"}},
	"catalog":   {fields: []string{"workspace"}, stepNew: []string{"workspace"}},
}

// databricksFake fakes P0's install API for Databricks items, which P0 stores
// and checks as install-api.ts and configure.ts in the app do. The shared fake
// stores the items. It has no hook that can refuse a step, so the provider and
// the tests call a server in front of it, which answers a refused verify or
// configure step itself and passes every other request on.
type databricksFake struct {
	*fakeP0
	// The server in front of the shared fake's.
	front *httptest.Server
	// Whether the step that moves the item to nextState fails its install check,
	// as when a connector can't reach what the item names. P0 then saves nothing.
	refuse func(key fakeItemKey, nextState string) bool
}

// newDatabricksFake's fake trims each field, drops any field outside the
// component's schema, so that a field the provider misnames reads back empty,
// sets a catalog's workspace from its key when it stages the catalog, as the
// app's assembler does, and refuses a change to a `step: "new"` field after
// staging. It runs no install checks, so it can't tell whether the AWS or
// Databricks side of an install exists. A test fails a check with the fake's
// refuse hook.
func newDatabricksFake(t *testing.T) *databricksFake {
	f := &databricksFake{
		fakeP0: newFakeP0(t),
		refuse: func(fakeItemKey, string) bool { return false },
	}
	f.normalize = func(key fakeItemKey, item map[string]any) {
		schema, ok := databricksSchema[key.component]
		if key.integration != "databricks" || !ok {
			t.Errorf("unexpected item %v in the fake Databricks install", key)
			return
		}
		for field, value := range item {
			switch {
			case field == "state" || field == "label":
			case !slices.Contains(schema.fields, field):
				delete(item, field)
			default:
				if text, ok := value.(string); ok {
					item[field] = strings.TrimSpace(text)
				}
			}
		}
		// Whatever workspace staging sends, P0 saves the one that the key names.
		if key.component == "catalog" && item["state"] == "stage" {
			if at := strings.LastIndex(key.id, "@"); at >= 0 {
				item["workspace"] = key.id[at+1:]
			}
		}
	}

	api := f.server.Config.Handler
	item := "/o/" + fakeOrg + "/integrations/{integration}/config/{component}/{id}"
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+item+"/verify", f.checked("configure", api))
	mux.HandleFunc("POST "+item+"/configure", f.checked("installed", api))
	mux.Handle("/", api)
	f.front = httptest.NewServer(mux)
	t.Cleanup(f.front.Close)
	return f
}

// Answers the step that moves an item to nextState with a 422, P0's StateError,
// if P0 would refuse it, and otherwise passes the request on to api, the shared
// fake. P0 refuses before it saves the item, so a refused step changes nothing.
func (f *databricksFake) checked(nextState string, api http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		key := itemKeyOf(r)
		previous := f.item(key.integration, key.component, key.id)
		var sent map[string]any
		// The shared fake answers a step on an item it doesn't have, and a body that
		// isn't a JSON object.
		if previous == nil || json.Unmarshal(body, &sent) != nil {
			api.ServeHTTP(w, r)
			return
		}

		// The item as the shared fake would save it.
		updated := deepMerge(previous, sent)
		updated["state"] = nextState
		f.normalize(key, updated)
		for _, field := range databricksSchema[key.component].stepNew {
			if !reflect.DeepEqual(updated[field], previous[field]) {
				databricksUnprocessable(w, fmt.Sprintf("'%s' can only be altered on initial installation. Create a new installation to change this field.", field))
				return
			}
		}
		if f.refuse(key, nextState) {
			databricksUnprocessable(w, "The install check failed")
			return
		}
		api.ServeHTTP(w, r)
	}
}

func databricksUnprocessable(w http.ResponseWriter, message string) {
	body, _ := json.Marshal(map[string]string{"error": message})
	http.Error(w, string(body), http.StatusUnprocessableEntity)
}

// Configures the provider to call the fake.
func (f *databricksFake) providerConfig() string {
	return fmt.Sprintf(`
provider "p0" {
  org       = %q
  host      = %q
  api_token = "test-token"
}
`, fakeOrg, f.front.URL)
}

// A client of the fake's API, for checks that read P0's items directly, as tests
// against a real organization read them.
func (f *databricksFake) client() *internal.P0ProviderData {
	return &internal.P0ProviderData{BaseUrl: databricksOrgUrl(f.front.URL, fakeOrg), Client: f.front.Client()}
}

// The base URL of an organization's API, as the provider builds it.
func databricksOrgUrl(host string, org string) string {
	return fmt.Sprintf("%s/o/%s", host, org)
}
