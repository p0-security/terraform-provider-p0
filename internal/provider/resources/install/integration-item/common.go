// Package installintegrationitem manages any P0 install item through the generic
// install API, with its configuration passed as raw JSON.
//
// It exists for integrations that have no typed resource in this provider. Nothing
// here knows an integration's schema, so P0 validates the configuration only when
// it is applied.
package installintegrationitem

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

const unsupportedWarning = `**Unsupported.** This resource sends raw JSON to P0's install API without any
schema validation in Terraform. Prefer a typed resource for the integration whenever one exists, and only use this
one when P0 support has given you the configuration to apply.`

const limitations = `**Limitations:**

- ` + "`config`" + ` is merged into the item P0 already stores, so removing a key from ` + "`config`" + ` does not remove
  it from P0.
- Only the keys set in ` + "`config`" + ` are checked for drift. Keys that P0 drops or rewrites (for example, keys the
  integration does not define) show as a permanent difference.
- Fields that P0 only accepts on a new install can not be changed in place; change ` + "`id`" + ` to replace the item.
- Integrations that need a root installation (for example ` + "`gcloud`" + `) must already be installed, e.g. with
  ` + "`p0_gcp`" + `.
- Do not put secrets in ` + "`config`" + `: they are stored in plain text in the Terraform state.`

// The response to every install-item request: the stored item, plus any metadata the
// component computes from it (e.g. the cloud role P0 expects to assume).
//
// The item stays raw so that it can be decoded with json.Number, keeping numbers
// exactly as P0 wrote them.
type itemApi struct {
	Item     json.RawMessage            `json:"item"`
	Metadata map[string]json.RawMessage `json:"metadata"`
}

// The identity of an install item, shared by both resources' models.
type itemKey struct {
	Integration types.String
	Component   types.String
	Id          types.String
}

// Builds an installer for one resource instance. The common installer fixes the
// integration and component when the provider is configured, but here they are
// attributes of each instance, so a new installer is built per request.
//
// fromJson receives P0's decoded item; it is closed over by the caller so it can see
// the instance's own configuration, which the common installer does not pass along.
func newInstaller(
	data *internal.P0ProviderData,
	key itemKey,
	config types.String,
	fromJson func(ctx context.Context, diags *diag.Diagnostics, item map[string]any, api *itemApi) any,
) *common.Install {
	// The item id is free-form, and an id containing "/" or "?" would otherwise address
	// a different path.
	escapedId := url.PathEscape(key.Id.ValueString())
	return &common.Install{
		Integration:  url.PathEscape(key.Integration.ValueString()),
		Component:    url.PathEscape(key.Component.ValueString()),
		ProviderData: data,
		GetId: func(any) *string {
			return &escapedId
		},
		GetItemJson: func(json any) any {
			api, ok := json.(*itemApi)
			if !ok || len(api.Item) == 0 || string(api.Item) == "null" {
				return nil
			}
			return api
		},
		FromJson: func(ctx context.Context, diags *diag.Diagnostics, _ string, json any) any {
			api, ok := json.(*itemApi)
			if !ok {
				return nil
			}
			item, err := decodeObject(api.Item)
			if err != nil {
				diags.AddError("Bad API response", fmt.Sprintf("Could not read the install item from P0: %s", err))
				return nil
			}
			return fromJson(ctx, diags, item, api)
		},
		ToJson: func(any) any {
			body, err := configBody(config)
			if err != nil {
				return nil
			}
			return body
		},
	}
}

// PUTs the configuration to P0, which (re)assembles the item and leaves it staged.
func stage(ctx context.Context, diags *diag.Diagnostics, installer *common.Install, plan *tfsdk.Plan, state *tfsdk.State, model any) {
	body := installer.ToJson(model)
	if body == nil {
		diags.AddAttributeError(path.Root("config"), "Invalid JSON object", "'config' must be a JSON object")
		return
	}
	var json itemApi
	installer.Stage(ctx, diags, plan, state, &json, model, body)
}

// The request body P0 merges into the stored item. An unset config sends an empty
// object, which leaves the item as it is.
func configBody(config types.String) (map[string]any, error) {
	if config.IsNull() || config.IsUnknown() {
		return map[string]any{}, nil
	}
	return decodeObject([]byte(config.ValueString()))
}

// Decodes a JSON object, keeping numbers as json.Number so that re-encoding them does
// not change their representation (e.g. a large integer becoming 1e+21).
func decodeObject(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if decoder.More() {
		return nil, fmt.Errorf("unexpected data after the JSON object")
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("must be a JSON object")
	}
	return object, nil
}

// Encodes like Terraform's jsonencode (compact, with sorted object keys), so that a
// value read back from P0 compares equal to the configuration that wrote it.
func encodeCompact(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// Decides what to keep in state for `config` after reading the item from P0.
//
// Only the keys the configuration sets are compared, recursively, because P0 adds its
// own (state, label, defaults, computed fields). If those keys still hold the same
// values the prior string is kept, so that whitespace or key order never produces a
// diff. Otherwise the configured keys are re-encoded with P0's values, and the plan
// shows the drift.
func reconcileConfig(prior types.String, item map[string]any) (types.String, error) {
	if prior.IsNull() || prior.IsUnknown() {
		return prior, nil
	}
	configured, err := decodeObject([]byte(prior.ValueString()))
	if err != nil {
		return prior, err
	}
	current := restrictTo(configured, item)
	if reflect.DeepEqual(configured, current) {
		return prior, nil
	}
	encoded, err := encodeCompact(current)
	if err != nil {
		return prior, err
	}
	return types.StringValue(encoded), nil
}

// Computes the `config` and `item` attributes from the item P0 returned.
//
// Only Read reconciles `config`. Create and Update must store the planned value
// verbatim: Terraform rejects an apply whose result differs from its plan, and P0 may
// normalize what it stores (trimming strings, filling in defaults).
func itemValues(diags *diag.Diagnostics, config types.String, item map[string]any, reconcile bool) (types.String, types.String, bool) {
	if reconcile {
		var err error
		config, err = reconcileConfig(config, item)
		if err != nil {
			diags.AddError("Invalid configuration in state", fmt.Sprintf("Could not compare 'config' with P0's item: %s", err))
			return config, types.StringNull(), false
		}
	}
	encoded, err := encodeCompact(item)
	if err != nil {
		diags.AddError("Bad API response", fmt.Sprintf("Could not encode the install item: %s", err))
		return config, types.StringNull(), false
	}
	return config, types.StringValue(encoded), true
}

// Returns the parts of actual that shape describes: the keys of every nested object in
// shape, with actual's values. A key missing from actual is left out.
func restrictTo(shape map[string]any, actual map[string]any) map[string]any {
	restricted := map[string]any{}
	for key, shapeValue := range shape {
		actualValue, ok := actual[key]
		if !ok {
			continue
		}
		shapeObject, shapeIsObject := shapeValue.(map[string]any)
		actualObject, actualIsObject := actualValue.(map[string]any)
		if shapeIsObject && actualIsObject {
			restricted[key] = restrictTo(shapeObject, actualObject)
		} else {
			restricted[key] = actualValue
		}
	}
	return restricted
}

// Converts component metadata to a map of strings. Metadata values are strings in
// practice (often JSON policy documents); any other value is kept as its raw JSON.
// The result is never null, so that components without metadata do not flip between
// null and empty.
func metadataMap(ctx context.Context, diags *diag.Diagnostics, metadata map[string]json.RawMessage) types.Map {
	values := map[string]string{}
	for key, raw := range metadata {
		if string(raw) == "null" {
			continue
		}
		var str string
		if err := json.Unmarshal(raw, &str); err == nil {
			values[key] = str
		} else {
			values[key] = string(raw)
		}
	}
	mapValue, mapDiags := types.MapValueFrom(ctx, types.StringType, values)
	diags.Append(mapDiags...)
	return mapValue
}

// Splits an import ID of the form "<integration>/<component>/<id>". The item id is
// last so that it may itself contain "/".
func parseImportId(importId string) (integration, component, id string, err error) {
	parts := strings.SplitN(importId, "/", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("expected an import ID of the form <integration>/<component>/<id>, got %q", importId)
	}
	return parts[0], parts[1], parts[2], nil
}

func importState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	integration, component, id, err := parseImportId(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("integration"), integration)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("component"), component)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// The attributes both resources share.
func keyAttributes() map[string]schema.Attribute {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	return map[string]schema.Attribute{
		"integration": schema.StringAttribute{
			MarkdownDescription: `The P0 integration key, e.g. ` + "`aws`",
			Required:            true,
			PlanModifiers:       requiresReplace,
		},
		"component": schema.StringAttribute{
			MarkdownDescription: `The integration's install component, e.g. ` + "`iam-write`",
			Required:            true,
			PlanModifiers:       requiresReplace,
		},
		"id": schema.StringAttribute{
			MarkdownDescription: `The install item's identifier within the component`,
			Required:            true,
			PlanModifiers:       requiresReplace,
		},
		"config": schema.StringAttribute{
			MarkdownDescription: `The item's configuration, as a JSON object (use ` + "`jsonencode`" + `). Sent to P0 as-is.`,
			Optional:            true,
			Validators:          []validator.String{jsonObjectValidator{}},
		},
		"item": schema.StringAttribute{
			MarkdownDescription: `The full item P0 stores, as JSON. Includes fields P0 computes, as well as ` + "`state`" + ` and ` + "`label`",
			Computed:            true,
		},
	}
}

type jsonObjectValidator struct{}

func (jsonObjectValidator) Description(context.Context) string {
	return "value must be a JSON object"
}

func (v jsonObjectValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (jsonObjectValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := decodeObject([]byte(req.ConfigValue.ValueString())); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid JSON object", fmt.Sprintf("'config' must be a JSON object: %s", err))
	}
}
