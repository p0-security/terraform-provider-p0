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

// A copy of databricks_ai_fake_test.go, for Redshift (AI).

// Each Redshift (AI) component's install schema, as the app defines it in
// packages/integrations/redshift-ai/shared/src/components.ts: its fields, and
// those marked `step: "new"`, which P0 refuses to change after staging. The
// hidden `warnings`, which only P0 sets, is left out.
var redshiftAiSchema = map[string]struct{ fields, stepNew []string }{
	"connector": {fields: []string{"region"}, stepNew: []string{"region"}},
	"cluster": {
		fields:  []string{"connector", "type", "database", "objectOwner", "arn"},
		stepNew: []string{"connector", "type"},
	},
}

// redshiftAiFake fakes P0's install API for Redshift (AI) items, which P0
// stores and checks as install-api.ts and configure.ts in the app do. The
// shared fake stores the items. Configuring a cluster sets its ARN, as the
// app's `arn` field hook does. It has no hook that can refuse a step, so a
// server in front of it answers a refused verify or configure step itself, and
// passes every other request on.
type redshiftAiFake struct {
	*fakeP0
	// Whether the step that moves the item to nextState fails its install check,
	// as when a connector can't reach what the item names. P0 then saves nothing.
	refuse func(key fakeItemKey, nextState string) bool
}

// newRedshiftAiFake returns a fake that trims each field, drops any field
// outside the component's schema, so that a field the provider misnames reads
// back empty, and refuses a change to a `step: "new"` field after staging. It
// runs no install checks, so it can't tell whether the AWS or Redshift side
// of an install exists. A test fails a check with the fake's refuse hook.
func newRedshiftAiFake(t *testing.T) *redshiftAiFake {
	f := &redshiftAiFake{
		fakeP0: newFakeP0(t),
		refuse: func(fakeItemKey, string) bool { return false },
	}
	f.normalize = func(key fakeItemKey, item map[string]any) {
		schema, ok := redshiftAiSchema[key.component]
		if key.integration != "redshift-ai" || !ok {
			t.Errorf("unexpected item %v in the fake Redshift (AI) install", key)
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
	}

	api := f.server.Config.Handler
	item := "/o/" + fakeOrg + "/integrations/{integration}/config/{component}/{id}"
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+item+"/verify", f.refusing("configure", api))
	mux.HandleFunc("POST "+item+"/configure", f.refusing("installed", api))
	mux.Handle("/", api)
	front := httptest.NewServer(mux)
	t.Cleanup(front.Close)
	// The provider and the tests find the fake by its server, so the server in
	// front takes its place. The shared fake's own server stays up behind it, and
	// closes with the test.
	f.server = front
	return f
}

// Answers the step that moves an item to nextState with a 422, P0's StateError,
// if P0 would refuse it, and otherwise passes the request on to api, the shared
// fake. P0 refuses before it saves the item, so a refused step changes nothing.
// A step that the shared fake would itself refuse, for an item in the wrong
// state, gets the 422 first.
func (f *redshiftAiFake) refusing(nextState string, api http.Handler) http.HandlerFunc {
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

		// The item's fields as the shared fake would save them.
		updated := deepMerge(previous, sent)
		updated["state"] = nextState
		f.normalize(key, updated)
		for _, field := range redshiftAiSchema[key.component].stepNew {
			if !reflect.DeepEqual(updated[field], previous[field]) {
				redshiftAiUnprocessable(w, fmt.Sprintf("'%s' can only be altered on initial installation. Create a new installation to change this field.", field))
				return
			}
		}
		if f.refuse(key, nextState) {
			redshiftAiUnprocessable(w, "The install check failed")
			return
		}
		if key.component == "cluster" && nextState == "installed" {
			sent["arn"] = redshiftAiArn(key.id)
			body, _ = json.Marshal(sent)
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
		}
		api.ServeHTTP(w, r)
	}
}

// The ARN the fake gives the cluster clusterId once it is configured. The app
// finds a provisioned cluster's or a workgroup's real ARN; any value that comes
// from the key shows that the provider reads it.
func redshiftAiArn(clusterId string) string {
	return "arn:aws:redshift:" + clusterId
}

func redshiftAiUnprocessable(w http.ResponseWriter, message string) {
	body, _ := json.Marshal(map[string]string{"error": message})
	http.Error(w, string(body), http.StatusUnprocessableEntity)
}

// Configures the provider to call the fake.
func (f *redshiftAiFake) providerConfig() string {
	return providerConfig(f.fakeP0)
}

// A client of the fake's API, for checks that read P0's items directly, as tests
// against a real organization read them.
func (f *redshiftAiFake) client() *internal.P0ProviderData {
	return &internal.P0ProviderData{BaseUrl: redshiftAiOrgUrl(f.server.URL, fakeOrg), Client: f.server.Client()}
}

// The base URL of an organization's API, as the provider builds it.
func redshiftAiOrgUrl(host string, org string) string {
	return fmt.Sprintf("%s/o/%s", host, org)
}
