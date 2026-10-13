package common

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/p0-security/terraform-provider-p0/internal"
)

// A DescribeError that records what it was called with, and returns summary.
type describeCall struct {
	called bool
	id     string
	err    error
}

func recordingDescribe(summary string) (DescribeError, *describeCall) {
	call := &describeCall{}
	return func(id string, err error) (string, string) {
		*call = describeCall{called: true, id: id, err: err}
		return summary, "detail"
	}, call
}

// Checks that a hook was called for the item "item", with P0's error.
func (c *describeCall) check(t *testing.T) {
	t.Helper()
	if !c.called || c.id != "item" || c.err == nil {
		t.Errorf("hook called = %v with id %q and err %v; want it called with id %q and an error", c.called, c.id, c.err, "item")
	}
}

// A failed install check on either step goes to DescribeCheckError, a step that finds
// no item goes to DescribeWriteNotFound or says so, and every other failure keeps the
// generic diagnostic.
func TestStepError(t *testing.T) {
	const generic = "Error communicating with P0"
	const described = "described"
	const notFound = "Could not verify component component"
	describe := func(id string, err error) (string, string) { return described, id }
	status := func(code int) *http.Response { return &http.Response{StatusCode: code} }

	const notFoundDescribed = "not found described"
	cases := []struct {
		name     string
		describe DescribeError
		notFound bool
		step     string
		resp     *http.Response
		want     string
	}{
		{name: "verify check failed", describe: describe, step: Verify, resp: status(http.StatusBadRequest), want: described},
		{name: "verify check unprocessable", describe: describe, step: Verify, resp: status(http.StatusUnprocessableEntity), want: described},
		{name: "verify connector unreachable", describe: describe, step: Verify, resp: status(http.StatusBadGateway), want: described},
		{name: "configure check failed", describe: describe, step: Config, resp: status(http.StatusBadRequest), want: described},
		{name: "configure check unprocessable", describe: describe, step: Config, resp: status(http.StatusUnprocessableEntity), want: described},
		{name: "configure connector unreachable", describe: describe, step: Config, resp: status(http.StatusBadGateway), want: described},
		{name: "check failed without a describer", step: Verify, resp: status(http.StatusUnprocessableEntity), want: generic},
		{name: "unauthenticated", describe: describe, step: Verify, resp: status(http.StatusUnauthorized), want: generic},
		{name: "unauthorized", describe: describe, step: Config, resp: status(http.StatusForbidden), want: generic},
		{name: "rate limited", describe: describe, step: Verify, resp: status(http.StatusTooManyRequests), want: generic},
		{name: "server error", describe: describe, step: Verify, resp: status(http.StatusInternalServerError), want: generic},
		{name: "never reached P0", describe: describe, step: Verify, resp: nil, want: generic},
		{name: "item not found", describe: describe, step: Verify, resp: status(http.StatusNotFound), want: notFound},
		{name: "item not found without a describer", step: Verify, resp: status(http.StatusNotFound), want: notFound},
		{name: "verify not found with a not-found describer", describe: describe, notFound: true, step: Verify, resp: status(http.StatusNotFound), want: notFoundDescribed},
		{name: "configure not found with a not-found describer", notFound: true, step: Config, resp: status(http.StatusNotFound), want: notFoundDescribed},
		{name: "check failed with a not-found describer", describe: describe, notFound: true, step: Verify, resp: status(http.StatusUnprocessableEntity), want: described},
		{name: "server error with a not-found describer", notFound: true, step: Verify, resp: status(http.StatusInternalServerError), want: generic},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			install := &Install{Component: "component", DescribeCheckError: c.describe}
			var notFoundCall *describeCall
			if c.notFound {
				install.DescribeWriteNotFound, notFoundCall = recordingDescribe(notFoundDescribed)
			}
			summary, detail := install.stepError("item", c.step, c.resp, errors.New("boom"))
			if summary == notFoundDescribed {
				notFoundCall.check(t)
			}
			if summary != c.want {
				t.Errorf("summary = %q; want %q", summary, c.want)
			}
			if summary == described && detail != "item" {
				t.Errorf("DescribeCheckError got id %q; want %q", detail, "item")
			}
		})
	}
}

// A 404 from staging goes to DescribeWriteNotFound. Every other failure, and a 404
// without the hook, keeps the generic diagnostic, which carries P0's message.
func TestStageError(t *testing.T) {
	const generic = "Could not stage component component"
	const described = "described"
	status := func(code int) *http.Response { return &http.Response{StatusCode: code} }

	cases := []struct {
		name     string
		describe bool
		resp     *http.Response
		want     string
	}{
		{name: "not found", describe: true, resp: status(http.StatusNotFound), want: described},
		{name: "not found without a describer", resp: status(http.StatusNotFound), want: generic},
		{name: "refused value", describe: true, resp: status(http.StatusBadRequest), want: generic},
		{name: "conflict", describe: true, resp: status(http.StatusConflict), want: generic},
		{name: "unauthorized", describe: true, resp: status(http.StatusForbidden), want: generic},
		{name: "never reached P0", describe: true, resp: nil, want: generic},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			install := &Install{Component: "component"}
			var call *describeCall
			if c.describe {
				install.DescribeWriteNotFound, call = recordingDescribe(described)
			}
			summary, detail := install.stageError("item", c.resp, errors.New("boom"))
			if summary != c.want {
				t.Errorf("stageError() summary = %q; want %q", summary, c.want)
			}
			switch {
			case summary == described:
				call.check(t)
			case detail != "Error: boom":
				t.Errorf("stageError() detail = %q; want P0's error as before", detail)
			}
		})
	}
}

type testItemModel struct {
	Id    types.String `tfsdk:"id"`
	State types.String `tfsdk:"state"`
}

type testItemJson struct {
	State *string `json:"state"`
}

type testItemApi struct {
	Item *testItemJson `json:"item"`
}

func newTestInstall(baseUrl string, client *http.Client) *Install {
	return &Install{
		Integration:  "integration",
		Component:    "component",
		ProviderData: &internal.P0ProviderData{BaseUrl: baseUrl, Authentication: "Bearer x", Client: client},
		GetId: func(data any) *string {
			model, ok := data.(*testItemModel)
			if !ok {
				return nil
			}
			id := model.Id.ValueString()
			return &id
		},
		GetItemJson: func(json any) any {
			api, ok := json.(*testItemApi)
			if !ok {
				return nil
			}
			return api.Item
		},
		FromJson: func(_ context.Context, _ *diag.Diagnostics, id string, json any) any {
			item, ok := json.(*testItemJson)
			if !ok {
				return nil
			}
			return &testItemModel{Id: types.StringValue(id), State: types.StringPointerValue(item.State)}
		},
		ToJson: func(any) any { return &struct{}{} },
	}
}

var testItemSchema = schema.Schema{Attributes: map[string]schema.Attribute{
	"id":    schema.StringAttribute{Required: true},
	"state": schema.StringAttribute{Computed: true},
}}

// The item "item" at state, as Terraform holds it.
func testItemValue(t *testing.T, state any) tftypes.Value {
	t.Helper()
	objectType, ok := testItemSchema.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is not an object")
	}
	return tftypes.NewValue(objectType, map[string]tftypes.Value{
		"id":    tftypes.NewValue(tftypes.String, "item"),
		"state": tftypes.NewValue(tftypes.String, state),
	})
}

// A fake of P0's install API, which answers every request with respond's status and
// body, and records each request's method and path.
type recordingServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []string
}

func newRecordingServer(t *testing.T, respond func(r *http.Request) (int, string)) *recordingServer {
	t.Helper()
	s := &recordingServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, r.Method+" "+r.URL.Path)
		s.mu.Unlock()
		status, body := respond(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *recordingServer) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.requests...)
}

// Answers each step with the item at the state that the step moves it to.
func advance(r *http.Request) (int, string) {
	if strings.HasSuffix(r.URL.Path, "/"+Verify) {
		return http.StatusOK, `{"ok":true,"item":{"state":"configure"}}`
	}
	return http.StatusOK, `{"ok":true,"item":{"state":"installed"}}`
}

const testItemPath = "/integrations/integration/config/component/item"

// Create calls UpsertFromStage right after Stage. A step that finds no item fails the
// apply with an error that says so, and leaves the staged resource in state, so that
// Terraform replaces it on the next apply instead of losing track of it.
func TestUpsertFromStageKeepsStateOnNotFound(t *testing.T) {
	server := newRecordingServer(t, func(*http.Request) (int, string) {
		return http.StatusNotFound, `{"error":"Not found"}`
	})

	ctx := context.Background()
	plan := tfsdk.Plan{Schema: testItemSchema, Raw: testItemValue(t, tftypes.UnknownValue)}
	staged := tfsdk.State{Schema: testItemSchema, Raw: testItemValue(t, StateStage)}

	var diags diag.Diagnostics
	newTestInstall(server.URL, server.Client()).UpsertFromStage(ctx, &diags, &plan, &staged, &testItemApi{}, &testItemModel{})

	errs := diags.Errors()
	if len(errs) != 1 || errs[0].Summary() != "Could not verify component component" || !strings.Contains(errs[0].Detail(), `P0 has no component item "item"`) {
		t.Fatalf("diagnostics = %v; want one error saying the item is missing", diags)
	}
	if !staged.Raw.Equal(testItemValue(t, StateStage)) {
		t.Errorf("state = %v; want the staged state kept", staged.Raw)
	}
}

// UpsertFromStage verifies the item, then configures it. UpsertFromConfigure only
// configures it, since P0 has verified it already.
func TestUpsertSteps(t *testing.T) {
	verify := "POST " + testItemPath + "/verify"
	configure := "POST " + testItemPath + "/configure"
	cases := []struct {
		name   string
		upsert func(*Install, context.Context, *diag.Diagnostics, *tfsdk.Plan, *tfsdk.State, any, any)
		prior  string
		want   []string
	}{
		{name: "from stage", upsert: (*Install).UpsertFromStage, prior: StateStage, want: []string{verify, configure}},
		{name: "from configure", upsert: (*Install).UpsertFromConfigure, prior: StateConfigure, want: []string{configure}},
		{name: "from installed", upsert: (*Install).UpsertFromConfigure, prior: StateInstalled, want: []string{configure}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server := newRecordingServer(t, advance)
			plan := tfsdk.Plan{Schema: testItemSchema, Raw: testItemValue(t, tftypes.UnknownValue)}
			state := tfsdk.State{Schema: testItemSchema, Raw: testItemValue(t, c.prior)}

			var diags diag.Diagnostics
			c.upsert(newTestInstall(server.URL, server.Client()), context.Background(), &diags, &plan, &state, &testItemApi{}, &testItemModel{})

			if diags.HasError() {
				t.Fatalf("diagnostics = %v", diags)
			}
			if got := server.Requests(); !reflect.DeepEqual(got, c.want) {
				t.Errorf("requests = %v; want %v", got, c.want)
			}
			if !state.Raw.Equal(testItemValue(t, StateInstalled)) {
				t.Errorf("state = %v; want the installed item", state.Raw)
			}
		})
	}
}

// P0 saves nothing when the configure step fails, so neither does UpsertFromConfigure:
// an Update that fails keeps the item's state, which the framework starts as the prior
// state.
func TestUpsertFromConfigureKeepsStateOnFailure(t *testing.T) {
	server := newRecordingServer(t, func(*http.Request) (int, string) {
		return http.StatusBadGateway, `{"error":"integration is temporarily unavailable; please try again later"}`
	})
	plan := tfsdk.Plan{Schema: testItemSchema, Raw: testItemValue(t, tftypes.UnknownValue)}
	installed := tfsdk.State{Schema: testItemSchema, Raw: testItemValue(t, StateInstalled)}

	var diags diag.Diagnostics
	newTestInstall(server.URL, server.Client()).UpsertFromConfigure(context.Background(), &diags, &plan, &installed, &testItemApi{}, &testItemModel{})

	if errs := diags.Errors(); len(errs) != 1 || !strings.Contains(errs[0].Detail(), "Could not configure component component") {
		t.Fatalf("diagnostics = %v; want one error from the configure step", diags)
	}
	if got, want := server.Requests(), []string{"POST " + testItemPath + "/configure"}; !reflect.DeepEqual(got, want) {
		t.Errorf("requests = %v; want %v", got, want)
	}
	if !installed.Raw.Equal(testItemValue(t, StateInstalled)) {
		t.Errorf("state = %v; want the installed state kept", installed.Raw)
	}
}

// A read that finds no item removes the resource from state, so that the next apply
// creates it again. With FailReadOnNotFound set, it fails instead, and keeps the
// resource.
func TestReadNotFound(t *testing.T) {
	server := newRecordingServer(t, func(*http.Request) (int, string) {
		return http.StatusNotFound, `{"error":"Not found"}`
	})

	t.Run("without a describer", func(t *testing.T) {
		state := tfsdk.State{Schema: testItemSchema, Raw: testItemValue(t, StateInstalled)}
		var diags diag.Diagnostics
		newTestInstall(server.URL, server.Client()).Read(context.Background(), &diags, &state, &testItemApi{}, &testItemModel{})

		if diags.HasError() {
			t.Fatalf("diagnostics = %v", diags)
		}
		if !state.Raw.IsNull() {
			t.Errorf("state = %v; want the resource removed", state.Raw)
		}
	})

	t.Run("with a describer", func(t *testing.T) {
		state := tfsdk.State{Schema: testItemSchema, Raw: testItemValue(t, StateInstalled)}
		install := newTestInstall(server.URL, server.Client())
		var call *describeCall
		install.FailReadOnNotFound, call = recordingDescribe("described")
		var diags diag.Diagnostics
		install.Read(context.Background(), &diags, &state, &testItemApi{}, &testItemModel{})

		errs := diags.Errors()
		if len(errs) != 1 || errs[0].Summary() != "described" {
			t.Fatalf("diagnostics = %v; want the described error", diags)
		}
		call.check(t)
		if !state.Raw.Equal(testItemValue(t, StateInstalled)) {
			t.Errorf("state = %v; want the resource kept", state.Raw)
		}
	})
}

// Stage, end to end, against a P0 that answers the PUT with a 404: the hook's
// diagnostic when it's set, the generic one when it isn't, and no state either way.
func TestStageNotFound(t *testing.T) {
	server := newRecordingServer(t, func(*http.Request) (int, string) {
		return http.StatusNotFound, `{"error":"Not found"}`
	})

	for _, c := range []struct {
		name     string
		describe bool
		want     string
	}{
		{name: "with a describer", describe: true, want: "described"},
		{name: "without a describer", want: "Could not stage component component"},
	} {
		t.Run(c.name, func(t *testing.T) {
			install := newTestInstall(server.URL, server.Client())
			var call *describeCall
			if c.describe {
				install.DescribeWriteNotFound, call = recordingDescribe("described")
			}
			plan := tfsdk.Plan{Schema: testItemSchema, Raw: testItemValue(t, tftypes.UnknownValue)}
			state := tfsdk.State{Schema: testItemSchema, Raw: tftypes.NewValue(testItemSchema.Type().TerraformType(context.Background()), nil)}

			var diags diag.Diagnostics
			install.Stage(context.Background(), &diags, &plan, &state, &testItemApi{}, &testItemModel{}, &struct{}{})

			if errs := diags.Errors(); len(errs) != 1 || errs[0].Summary() != c.want {
				t.Fatalf("diagnostics = %v; want one error %q", diags, c.want)
			}
			if c.describe {
				call.check(t)
			}
			if !state.Raw.IsNull() {
				t.Errorf("state = %v; want none", state.Raw)
			}
			if got := server.Requests(); len(got) == 0 || got[len(got)-1] != "PUT "+testItemPath {
				t.Errorf("requests = %v; want the PUT last", got)
			}
		})
	}
}
