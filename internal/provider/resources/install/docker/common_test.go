package installdocker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

func marshal(t *testing.T, value any) string {
	t.Helper()
	got, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(got)
}

func schemaOf(t *testing.T, r resource.Resource) schema.Schema {
	t.Helper()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func objectTypeOf(t *testing.T, s schema.Schema) tftypes.Object {
	t.Helper()
	objectType, ok := s.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is not an object")
	}
	return objectType
}

// An object with the given fields set and every other field null.
func objectOf(fields map[string]tftypes.Value) func(tftypes.Object) tftypes.Value {
	return func(typ tftypes.Object) tftypes.Value {
		values := map[string]tftypes.Value{}
		for name, attrType := range typ.AttributeTypes {
			values[name] = tftypes.NewValue(attrType, nil)
		}
		for name, value := range fields {
			values[name] = value
		}
		return tftypes.NewValue(typ, values)
	}
}

// An object that Terraform only knows at apply time, such as one set from a module
// output.
func unknownObject(typ tftypes.Object) tftypes.Value {
	return tftypes.NewValue(typ, tftypes.UnknownValue)
}

func str(value string) tftypes.Value {
	return tftypes.NewValue(tftypes.String, value)
}

var unknownString = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)

// A nested object with base's fields, one of them replaced by value.
func with(base map[string]tftypes.Value, field string, value tftypes.Value) func(tftypes.Object) tftypes.Value {
	fields := map[string]tftypes.Value{field: value}
	for name, baseValue := range base {
		if name != field {
			fields[name] = baseValue
		}
	}
	return objectOf(fields)
}

// A nested object with base's fields but the one named.
func without(base map[string]tftypes.Value, field string) func(tftypes.Object) tftypes.Value {
	fields := map[string]tftypes.Value{}
	for name, baseValue := range base {
		if name != field {
			fields[name] = baseValue
		}
	}
	return objectOf(fields)
}

// Runs a resource's ValidateConfig against a configuration with the given nested
// blocks, each built for its type, and string values, and every other attribute null.
// The attribute validators don't run here, because Terraform runs those separately.
func validateConfig(t *testing.T, r resource.ResourceWithValidateConfig, blocks map[string]func(tftypes.Object) tftypes.Value, values map[string]tftypes.Value) diag.Diagnostics {
	t.Helper()
	ctx := context.Background()
	s := schemaOf(t, r)
	objectType := objectTypeOf(t, s)

	fields := map[string]tftypes.Value{}
	for name, value := range values {
		fields[name] = value
	}
	for name, build := range blocks {
		typ, ok := objectType.AttributeTypes[name].(tftypes.Object)
		if !ok {
			t.Fatalf("%s is not an object", name)
		}
		fields[name] = build(typ)
	}

	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: s, Raw: objectOf(fields)(objectType)}}, resp)
	return resp.Diagnostics
}

// Each error as "<path>: <detail>".
func errorDetails(diags diag.Diagnostics) []string {
	got := []string{}
	for _, d := range diags.Errors() {
		withPath, ok := d.(diag.DiagnosticWithPath)
		if !ok {
			got = append(got, "<no path>: "+d.Detail())
			continue
		}
		got = append(got, fmt.Sprintf("%s: %s", withPath.Path(), d.Detail()))
	}
	sort.Strings(got)
	return got
}

func errorSummaries(diags diag.Diagnostics) []string {
	got := []string{}
	for _, d := range diags.Errors() {
		got = append(got, d.Summary())
	}
	sort.Strings(got)
	return got
}

// What attribute's validators report for value.
func validateString(t *testing.T, attribute schema.Attribute, value string) diag.Diagnostics {
	t.Helper()
	stringAttribute, ok := attribute.(schema.StringAttribute)
	if !ok {
		t.Fatalf("%v is not a string attribute", attribute)
	}
	resp := &validator.StringResponse{}
	for _, v := range stringAttribute.Validators {
		v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("value"), ConfigValue: types.StringValue(value)}, resp)
	}
	return resp.Diagnostics
}

// Whether attribute's validators accept value.
func accepts(t *testing.T, attribute schema.Attribute, value string) bool {
	t.Helper()
	return !validateString(t, attribute, value).HasError()
}

func nestedAttributes(t *testing.T, s schema.Schema, name string) map[string]schema.Attribute {
	t.Helper()
	attribute, ok := s.Attributes[name].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("%s is not a nested attribute", name)
	}
	return attribute.Attributes
}

func requiresReplace(t *testing.T, attribute schema.Attribute) bool {
	t.Helper()
	ctx := context.Background()
	stringAttribute, ok := attribute.(schema.StringAttribute)
	if !ok {
		t.Fatalf("%v is not a string attribute", attribute)
	}
	for _, modifier := range stringAttribute.PlanModifiers {
		if modifier.Description(ctx) == stringplanmodifier.RequiresReplace().Description(ctx) {
			return true
		}
	}
	return false
}

// model as r's state or plan holds it, or null for a nil model.
func rawOf(t *testing.T, r resource.Resource, model any) tftypes.Value {
	t.Helper()
	ctx := context.Background()
	s := schemaOf(t, r)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if model == nil || reflect.ValueOf(model).IsNil() {
		return state.Raw
	}
	if diags := state.Set(ctx, model); diags.HasError() {
		t.Fatalf("Set: %v", diags)
	}
	return state.Raw
}

// Decodes a state with every schema attribute null into the model, as the framework
// does on import.
func decodeNullState(t *testing.T, r resource.Resource, model any) {
	t.Helper()
	ctx := context.Background()
	s := schemaOf(t, r)
	state := tfsdk.State{Schema: s, Raw: objectOf(nil)(objectTypeOf(t, s))}
	if diags := state.Get(ctx, model); diags.HasError() {
		t.Fatalf("decode: %v", diags)
	}
}

// Each resource's schema is one the framework accepts, and its model has exactly the
// schema's attributes: a field the schema lacks, or an attribute the model lacks, fails
// every plan and apply with a conversion error that neither `go build` nor `go vet`
// catches.
func TestSchemasAndModels(t *testing.T) {
	cases := map[string]struct {
		resource resource.Resource
		model    any
	}{
		"connector staged": {resource: NewDockerConnectorStaged(), model: &dockerConnectorStagedModel{}},
		"connector":        {resource: NewDockerConnector(), model: &dockerConnectorModel{}},
		"organization":     {resource: NewDockerOrganization(), model: &dockerOrganizationModel{}},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if diags := schemaOf(t, c.resource).ValidateImplementation(context.Background()); diags.HasError() {
				t.Errorf("ValidateImplementation: %v", diags)
			}
			decodeNullState(t, c.resource, c.model)
		})
	}
}

func TestResourceNames(t *testing.T) {
	cases := map[string]resource.Resource{
		"p0_docker_connector_staged": NewDockerConnectorStaged(),
		"p0_docker_connector":        NewDockerConnector(),
		"p0_docker_organization":     NewDockerOrganization(),
	}

	for want, r := range cases {
		resp := &resource.MetadataResponse{}
		r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "p0"}, resp)
		if resp.TypeName != want {
			t.Errorf("type name = %q; want %q", resp.TypeName, want)
		}
	}
}

// P0's rule for a connector's ID (CONNECTOR_ID_PATTERN and CONNECTOR_ID_MAX_LENGTH),
// wherever a connector is named, and its words for an ID that breaks it
// (INVALID_CONNECTOR_ID).
func TestConnectorIdValidators(t *testing.T) {
	const message = "Use lowercase letters, digits and hyphens only, starting and ending with a letter or digit, up to 63 characters"

	cases := map[string]bool{
		"prod":                  true,
		"p":                     true,
		"0":                     true,
		"us-west-2":             true,
		strings.Repeat("a", 63): true,
		strings.Repeat("a", 64): false,
		"Prod":                  false,
		"-prod":                 false,
		"prod-":                 false,
		"prod_1":                false,
		"prod/1":                false,
		" prod":                 false,
		"":                      false,
	}

	attributes := map[string]schema.Attribute{
		"p0_docker_connector_staged.id":    schemaOf(t, NewDockerConnectorStaged()).Attributes["id"],
		"p0_docker_connector.id":           schemaOf(t, NewDockerConnector()).Attributes["id"],
		"p0_docker_organization.connector": schemaOf(t, NewDockerOrganization()).Attributes["connector"],
	}
	for name, attribute := range attributes {
		for id, want := range cases {
			if got := accepts(t, attribute, id); got != want {
				t.Errorf("%s accepts %q = %v; want %v", name, id, got, want)
			}
		}
		if got := errorDetails(validateString(t, attribute, "Prod")); len(got) != 1 || !strings.Contains(got[0], message) {
			t.Errorf("%s refuses an ID with %v; want P0's message", name, got)
		}
	}
}

// Docker Hub's rule for an organization's name, with P0's own cases for it
// (packages/integrations/docker/backend/src/install/__tests__/organization.test.ts in
// the app).
func TestOrganizationIdValidator(t *testing.T) {
	cases := map[string]bool{
		"p0security":             true,
		"my-docker-org":          true,
		"my_docker_org":          true,
		"0rg":                    true,
		"p0":                     true,
		strings.Repeat("a", 255): true,
		strings.Repeat("a", 256): false,
		"P0Security":             false,
		"-p0security":            false,
		"_p0security":            false,
		"p":                      false,
		"p0security/admins":      false,
		"..":                     false,
		"p0security ":            false,
		"":                       false,
	}

	attribute := schemaOf(t, NewDockerOrganization()).Attributes["id"]
	for org, want := range cases {
		if got := accepts(t, attribute, org); got != want {
			t.Errorf("id accepts %q = %v; want %v", org, got, want)
		}
	}
}

// The wording names the item and shows P0's message, without presuming that the
// connector is at fault.
func TestDescribeCheckError(t *testing.T) {
	const message = "422 Unprocessable Entity: P0 couldn't invoke the connector prod, the Lambda function p0-docker-prod in AWS account 123456789012, region us-west-2."
	cases := []struct {
		kind    string
		id      string
		summary string
		detail  string
	}{
		{
			kind:    "connector",
			id:      "prod",
			summary: "Docker Hub connector install check failed",
			detail:  "P0 rejected the install check for the Docker Hub connector \"prod\":\n\n" + message,
		},
		{
			kind:    "organization",
			id:      "my-docker-org",
			summary: "Docker Hub organization install check failed",
			detail:  "P0 rejected the install check for the Docker Hub organization \"my-docker-org\":\n\n" + message,
		},
	}

	for _, c := range cases {
		summary, detail := describeCheckError(c.kind)(c.id, errors.New(message))
		if summary != c.summary {
			t.Errorf("summary = %q; want %q", summary, c.summary)
		}
		if detail != c.detail {
			t.Errorf("detail =\n%s\nwant\n%s", detail, c.detail)
		}
	}
}

// A fake of P0's install API for the Docker Hub items, which records each request.
// Like P0, it refuses a PUT of an item that exists, answers 404 for a step on one that
// doesn't, merges a request over the stored item, resolves a Cloud Run connector's URL
// and reads an organization's allowed domains when it verifies, and names a
// connector's identity in its metadata. With failVerify, verify answers as P0 does when
// an install check fails, and saves nothing.
type fakeItems struct {
	failVerify bool

	mu       sync.Mutex
	items    map[string]map[string]any
	requests []string
}

const (
	basePath         = "/integrations/docker/config"
	connectorPath    = basePath + "/connector/prod"
	organizationPath = basePath + "/organization/my-docker-org"

	fakeServiceUri     = "https://p0-docker-prod-abc123-uw.a.run.app"
	fakeAllowedDomains = `^example\.com$`
)

func newFakeItems(t *testing.T) (*fakeItems, *httptest.Server) {
	t.Helper()
	fake := &fakeItems{items: map[string]map[string]any{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return fake, server
}

// Stores item at path, as an earlier request left it.
func (f *fakeItems) seed(t *testing.T, itemPath string, state string, item any) {
	t.Helper()
	stored := map[string]any{}
	if err := json.Unmarshal([]byte(marshal(t, item)), &stored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	stored["state"] = state
	f.complete(itemPath, stored)
	f.items[itemPath] = stored
}

// Adds what P0's installers assign once an item is verified.
func (f *fakeItems) complete(itemPath string, item map[string]any) {
	if item["state"] == common.StateStage {
		return
	}
	if hosting, ok := item["hosting"].(map[string]any); ok && hosting["type"] == "gcp" {
		hosting["connectorServiceUri"] = fakeServiceUri
	}
	if strings.HasPrefix(itemPath, basePath+"/organization/") {
		item["allowedDomains"] = fakeAllowedDomains
	}
}

func (f *fakeItems) metadata(item map[string]any) map[string]any {
	hosting, ok := item["hosting"].(map[string]any)
	if !ok {
		return nil
	}
	name := hosting["connectorName"]
	if hosting["type"] == "gcp" {
		return map[string]any{"connectorIdentity": map[string]any{
			"type":                "gcp",
			"projectId":           hosting["projectId"],
			"serviceAccountEmail": fmt.Sprintf("%s@%s.iam.gserviceaccount.com", name, hosting["projectId"]),
			"serviceAccountId":    name,
		}}
	}
	return map[string]any{"connectorIdentity": map[string]any{
		"type":      "aws",
		"accountId": hosting["accountId"],
		"roleArn":   fmt.Sprintf("arn:aws:iam::%s:role/%s", hosting["accountId"], name),
		"roleName":  name,
	}}
}

func (f *fakeItems) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.requests = append(f.requests, strings.TrimSpace(fmt.Sprintf("%s %s %s", r.Method, r.URL.Path, body)))

	w.Header().Set("Content-Type", "application/json")
	refuse := func(status int, message string) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": message})
	}

	if r.Method == http.MethodPost && r.URL.Path == basePath {
		refuse(http.StatusConflict, "Integration is already installed")
		return
	}

	itemPath, step := r.URL.Path, ""
	for _, name := range []string{common.Verify, common.Config} {
		if trimmed, ok := strings.CutSuffix(r.URL.Path, "/"+name); ok && r.Method == http.MethodPost {
			itemPath, step = trimmed, name
		}
	}
	stored, exists := f.items[itemPath]

	var nextState string
	switch {
	case r.Method == http.MethodDelete && !exists, step != "" && !exists:
		refuse(http.StatusNotFound, "Not found")
		return
	case r.Method == http.MethodDelete:
		delete(f.items, itemPath)
		w.WriteHeader(http.StatusNoContent)
		return
	case r.Method == http.MethodPut && exists:
		refuse(http.StatusConflict, "The item is already added. To change it, remove it and add it again.")
		return
	case r.Method == http.MethodPut:
		nextState = common.StateStage
	case step == common.Verify && f.failVerify:
		refuse(http.StatusUnprocessableEntity, "The connector isn't allowed to read the secret.")
		return
	case step == common.Verify:
		nextState = common.StateConfigure
	case step == common.Config:
		nextState = common.StateInstalled
	default:
		refuse(http.StatusBadRequest, "unexpected request")
		return
	}

	sent := map[string]any{}
	if err := json.Unmarshal(body, &sent); err != nil {
		refuse(http.StatusBadRequest, "request body must be a JSON object")
		return
	}
	updated := map[string]any{}
	for name, value := range stored {
		updated[name] = value
	}
	for name, value := range sent {
		updated[name] = value
	}
	updated["state"] = nextState
	f.complete(itemPath, updated)
	f.items[itemPath] = updated

	response := map[string]any{"ok": true, "item": updated}
	if metadata := f.metadata(updated); metadata != nil {
		response["metadata"] = metadata
	}
	_ = json.NewEncoder(w).Encode(response)
}

func (f *fakeItems) Requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.requests...)
}

// The state of the item at path, or "" when P0 has none.
func (f *fakeItems) State(itemPath string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, _ := f.items[itemPath]["state"].(string)
	return state
}

// r, configured against the fake P0 at server.
func configure[R resource.ResourceWithConfigure](t *testing.T, r R, server *httptest.Server) R {
	t.Helper()
	resp := &resource.ConfigureResponse{}
	data := internal.P0ProviderData{BaseUrl: server.URL, Authentication: "Bearer x", Client: server.Client()}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: data}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure: %v", resp.Diagnostics)
	}
	return r
}

// Runs Create with planned as the framework does, which starts the response's state as
// null.
func create(t *testing.T, r resource.Resource, planned tftypes.Value) *resource.CreateResponse {
	t.Helper()
	s := schemaOf(t, r)
	req := resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: planned}}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)}}
	r.Create(context.Background(), req, resp)
	return resp
}

// Runs Update from prior to planned as the framework does, which starts the response's
// state as the prior state.
func update(t *testing.T, r resource.Resource, prior, planned tftypes.Value) *resource.UpdateResponse {
	t.Helper()
	s := schemaOf(t, r)
	req := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: planned},
		State: tfsdk.State{Schema: s, Raw: prior},
	}
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: prior.Copy()}}
	r.Update(context.Background(), req, resp)
	return resp
}

// Runs Delete on prior as the framework does.
func destroy(t *testing.T, r resource.Resource, prior tftypes.Value) *resource.DeleteResponse {
	t.Helper()
	s := schemaOf(t, r)
	req := resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: prior}}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: prior.Copy()}}
	r.Delete(context.Background(), req, resp)
	return resp
}

// Runs ModifyPlan on the framework's plan for prior as the framework does, which starts
// the response's plan as its own.
func modifyPlan(t *testing.T, r resource.ResourceWithModifyPlan, prior, planned tftypes.Value) *resource.ModifyPlanResponse {
	t.Helper()
	s := schemaOf(t, r)
	req := resource.ModifyPlanRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: planned},
		State: tfsdk.State{Schema: s, Raw: prior},
	}
	resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: s, Raw: planned.Copy()}}
	r.ModifyPlan(context.Background(), req, resp)
	return resp
}

// When P0 answers a request with success but no item, each resource reports a bad
// response and saves nothing.
func TestCreateRefusesAResponseWithoutAnItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	cases := map[string]struct {
		resource resource.ResourceWithConfigure
		planned  any
	}{
		"connector staged": {resource: &DockerConnectorStaged{}, planned: awsCloud.plannedStaged()},
		"connector":        {resource: &DockerConnector{}, planned: awsCloud.plannedConnector()},
		"organization":     {resource: &DockerOrganization{}, planned: awsOrganization.planned()},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := configure(t, c.resource, server)

			resp := create(t, r, rawOf(t, r, c.planned))

			errs := resp.Diagnostics.Errors()
			if len(errs) != 1 || errs[0].Summary() != "Bad API response" || !strings.HasPrefix(errs[0].Detail(), "Could not read 'item' from") {
				t.Fatalf("diagnostics = %v; want the response refused for its missing item", resp.Diagnostics)
			}
			if !resp.State.Raw.IsNull() {
				t.Errorf("state = %v; want none", resp.State.Raw)
			}
		})
	}
}

func wantRequests(t *testing.T, fake *fakeItems, want ...string) {
	t.Helper()
	got := fake.Requests()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("requests =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
