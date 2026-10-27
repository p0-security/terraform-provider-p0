package installtailscale

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/p0-security/terraform-provider-p0/internal"
	"github.com/p0-security/terraform-provider-p0/internal/common"
)

// p0_tailscale_destination is new for INT-686. Its key, `<tailnet>:<name>`, copies the
// shape of internal/provider/resources/install/databricks/catalog.go: a computed `id`
// built from two fixed attributes, read back from the key, and an import that parses
// and checks the key.

var _ resource.Resource = &TailscaleDestination{}
var _ resource.ResourceWithConfigure = &TailscaleDestination{}
var _ resource.ResourceWithImportState = &TailscaleDestination{}
var _ resource.ResourceWithModifyPlan = &TailscaleDestination{}
var _ resource.ResourceWithValidateConfig = &TailscaleDestination{}

// The destination types, which are also the accesses a requestor asks for them with:
// the options of the app's `TailscaleDestinationKind` select element, in its order.
const (
	ipType           = "ip"
	deviceType       = "device"
	tagType          = "tag"
	serviceType      = "service"
	sshType          = "ssh"
	appConnectorType = "app-connector"
	capabilityType   = "capability"
	exitNodeType     = "exit-node"
	postureType      = "posture"
)

var destinationTypes = []string{ipType, deviceType, tagType, serviceType, sshType, appConnectorType, capabilityType, exitNodeType, postureType}

// What serves a Kubernetes API destination: the options of the capability's `proxy`
// select element, whose default is the operator.
const (
	operatorProxy   = "operator"
	proxyGroupProxy = "proxy-group"
)

type TailscaleDestination struct {
	installer *common.Install
}

type tailscaleDestinationModel struct {
	Id      types.String `tfsdk:"id"`
	Tailnet types.String `tfsdk:"tailnet"`
	Name    types.String `tfsdk:"name"`
	Type    types.String `tfsdk:"type"`
	Target  types.String `tfsdk:"target"`
	Ports   types.String `tfsdk:"ports"`
	SiteId  types.String `tfsdk:"site_id"`
	Via     types.String `tfsdk:"via"`
	Users   types.String `tfsdk:"users"`
	Groups  types.String `tfsdk:"groups"`
	Proxy   types.String `tfsdk:"proxy"`
	State   types.String `tfsdk:"state"`
}

func (m *tailscaleDestinationModel) key() string {
	return destinationKey(m.Tailnet.ValueString(), m.Name.ValueString())
}

// The fields of the destination's kind, by attribute name, for the rules that check
// them.
func (m *tailscaleDestinationModel) fields() map[string]types.String {
	return map[string]types.String{
		"target":  m.Target,
		"ports":   m.Ports,
		"site_id": m.SiteId,
		"via":     m.Via,
		"users":   m.Users,
		"groups":  m.Groups,
		"proxy":   m.Proxy,
	}
}

// The item: the tailnet its key names, which P0 sets from the key, and what it reaches,
// the `kind` select element. Each list, such as the ports, is one comma-separated
// string, as the app stores it.
type tailscaleDestinationJson struct {
	Tailnet *string   `json:"tailnet,omitempty"`
	Kind    *kindJson `json:"kind,omitempty"`
	State   *string   `json:"state,omitempty"`
}

type kindJson struct {
	Type   string      `json:"type"`
	Target *string     `json:"target,omitempty"`
	SiteId *string     `json:"siteId,omitempty"`
	Ports  *string     `json:"ports,omitempty"`
	Via    *string     `json:"via,omitempty"`
	Users  *string     `json:"users,omitempty"`
	Groups *string     `json:"groups,omitempty"`
	Proxy  *selectJson `json:"proxy,omitempty"`
}

type tailscaleDestinationApi struct {
	Item *tailscaleDestinationJson `json:"item"`
}

func NewTailscaleDestination() resource.Resource {
	return &TailscaleDestination{}
}

func (*TailscaleDestination) Metadata(_ context.Context, req resource.MetadataRequest, res *resource.MetadataResponse) {
	res.TypeName = req.ProviderTypeName + "_tailscale_destination"
}

// A required string that P0 fixes once the item exists: every field of a destination
// is `step: "new"` in the app's install schema. RequiresReplace plans the
// destroy-and-create that works, in place of an update that P0 would refuse. Copy of
// internal/provider/resources/install/databricks/schema.go's fixedAttribute.
func fixedAttribute(description string, validators ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: description,
		Validators:          validators,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

// An optional fixedAttribute: a field that only some types take.
func optionalFixedAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional:            true,
		MarkdownDescription: description,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

func (*TailscaleDestination) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Something requestors can get access to on a tailnet, such as a subnet behind a router, the devices with a tag, or a Kubernetes API. Each type is requested as its own access.

| ` + "`type`" + ` | ` + "`target`" + ` | Other attributes |
| --- | --- | --- |
| ` + "`ip`" + ` | An IPv4 or IPv6 CIDR inside an approved subnet route, such as ` + "`10.0.4.0/24`" + `. With ` + "`site_id`" + `, the IPv4 CIDR behind that 4via6 site. | ` + "`ports`" + ` (required), ` + "`site_id`" + `, ` + "`via`" + ` |
| ` + "`device`" + ` | The device's node ID, such as ` + "`nAbc123CNTRL`" + `, from the machine's page in the admin console | ` + "`ports`" + ` (required) |
| ` + "`tag`" + ` | A tag defined in ` + "`tagOwners`" + `, such as ` + "`tag:prod`" + `: every device with it | ` + "`ports`" + ` (required) |
| ` + "`service`" + ` | A Tailscale Service, ` + "`svc:`" + ` included, such as ` + "`svc:postgres`" + ` | ` + "`ports`" + ` (required) |
| ` + "`ssh`" + ` | A tag defined in ` + "`tagOwners`" + `: Tailscale SSH to every device with it, and port 22 on them | ` + "`users`" + ` |
| ` + "`app-connector`" + ` | The app connector's name in your policy file: internet traffic to its domains, through its devices | |
| ` + "`capability`" + ` | The tag of the Tailscale Kubernetes operator, or of its API server ProxyGroup, such as ` + "`tag:k8s-operator`" + `: the Kubernetes API, as Kubernetes groups | ` + "`groups`" + ` (required), ` + "`proxy`" + ` |
| ` + "`exit-node`" + ` | The node ID of a tagged exit node whose routes are approved: internet traffic through it | |
| ` + "`posture`" + ` | A P0 posture your policy file defines, such as ` + "`posture:p0_prod_db`" + `, whose assertion is ` + "`custom:p0_prod_db == true`" + ` | |

A tailnet in ` + "`posture`" + ` mode takes ` + "`posture`" + ` destinations only, and one in ` + "`direct-write`" + ` mode takes every other type.

**Important:** Creating this resource has P0 check the destination through the tailnet's connector: that its target is on the tailnet, and that a grant to it would render. If the check fails, the apply fails with P0's message. A posture destination's posture must be in the policy file first, so when ` + "`tailscale_acl`" + ` defines it, make the destination depend on that ` + "`tailscale_acl`" + `.

P0 fixes every field of a destination when it adds the destination, so changing any attribute replaces it. A grant that's already open keeps what it was made with, and P0 revokes exactly that. If the destination is already added in P0, import it by its ` + "`id`" + ` rather than creating it: P0 refuses to add a destination twice. If P0 hasn't finished installing it, for example because you imported it before its check passed, Terraform plans an update, and applying it finishes the install.

**Prerequisite:** the tailnet, installed with ` + "`p0_tailscale_tailnet`" + `.

**Note:** This integration is currently in beta.

For the full install steps, see [Tailscale](https://docs.p0.dev/integrations/resource-integrations/tailscale).`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The destination's key in P0, `<tailnet>:<name>`, such as `T1234CNTRL:prod-db`, which is also its import ID. Names are unique per tailnet only, so the key names the tailnet too.",
			},
			"tailnet": fixedAttribute(
				"The tailnet the destination is on: the `id` of its `p0_tailscale_tailnet`",
				tailnetIdValidator(),
			),
			"name": fixedAttribute(
				"A name you choose for the destination, unique on its tailnet, such as `prod-db`. Requestors ask for the destination by it. "+invalidDestinationName,
				destinationNameValidator(),
			),
			"type": fixedAttribute(
				"What the destination reaches: one of `ip`, `device`, `tag`, `service`, `ssh`, `app-connector`, `capability`, `exit-node` or `posture`",
				stringvalidator.OneOf(destinationTypes...),
			),
			"target": fixedAttribute(
				"What the destination reaches on the tailnet, in the form its `type` takes (see the table above)",
			),
			"ports": optionalFixedAttribute(
				"Comma-separated ports the grant opens, each a port, a range or `*`, optionally after a protocol, such as `tcp:5432`, `tcp:8000-8100`, `udp:53`, `icmp:*` or `*`. " +
					"Required for, and only valid with, `ip`, `device`, `tag` and `service`.",
			),
			"site_id": optionalFixedAttribute(
				"The 4via6 site ID, from 0 to 65535, when the range is behind a 4via6 subnet router. Only valid with `ip`.",
			),
			"via": optionalFixedAttribute(
				"The tag of the subnet router to go through, such as `tag:prod-router`. Leave it out to go through any router that serves the range. Only valid with `ip`.",
			),
			"users": optionalFixedAttribute(
				"Comma-separated users the requestor may log in as, such as `autogroup:nonroot` or `root`. Leave it out for `autogroup:nonroot`. Only valid with `ssh`.",
			),
			"groups": optionalFixedAttribute(
				"Comma-separated Kubernetes groups the requestor is impersonated as, such as `system:masters`. Required for, and only valid with, `capability`.",
			),
			"proxy": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "What serves the Kubernetes API: `operator` (the operator itself, the default) or `proxy-group` (a ProxyGroup, which also listens on port 80). " +
					"Only valid with `capability`, where P0 stores `operator` when it's left out.",
				Validators: []validator.String{
					stringvalidator.OneOf(operatorProxy, proxyGroupProxy),
				},
				PlanModifiers: []planmodifier.String{
					proxyDefault{},
					stringplanmodifier.RequiresReplace(),
				},
			},
			"state": common.StateAttribute,
		},
	}
}

// Plans the proxy P0 stores for a Kubernetes API destination that leaves it out, the
// select element's default, and none for any other type. P0 stores the default when
// it adds the destination, so planning it keeps the next plan empty, and taking `proxy`
// out of the configuration plans the default rather than keeping the old value.
type proxyDefault struct{}

var _ planmodifier.String = proxyDefault{}

func (proxyDefault) Description(context.Context) string {
	return "Defaults to `operator` for a `capability` destination, and is null for every other type."
}

func (m proxyDefault) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (proxyDefault) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if !req.ConfigValue.IsNull() {
		return
	}
	var destinationType types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("type"), &destinationType)...)
	switch {
	case resp.Diagnostics.HasError():
		return
	case destinationType.IsUnknown():
		resp.PlanValue = types.StringUnknown()
	case destinationType.ValueString() == capabilityType:
		resp.PlanValue = types.StringValue(operatorProxy)
	default:
		resp.PlanValue = types.StringNull()
	}
}

// The connector's rules for a destination's values (p0-connector-tailscale's
// src/policy/grants.ts, src/accesses/common/definition.ts and
// src/accesses/posture/definition.ts, on p0-security-perimeter's connector branch @
// 7ee943d), which the app checks a destination by when it adds it. P0's messages join
// the field's label and the connector's reason (`toConnectorDestination` in
// packages/integrations/tailscale/backend/src/destination.ts in the app).
var (
	tagPattern             = regexp.MustCompile(`^tag:[A-Za-z][A-Za-z0-9-]{0,62}$`)
	servicePattern         = regexp.MustCompile(`^svc:[A-Za-z0-9][A-Za-z0-9-]{0,62}$`)
	portPattern            = regexp.MustCompile(`^(?:\*|(?:(?:tcp|udp|sctp|icmp|\d{1,3}):)?(?:\*|\d{1,5}(?:-\d{1,5})?))$`)
	sshUserPattern         = regexp.MustCompile(`^[A-Za-z0-9_.:@*-]{1,64}$`)
	kubernetesGroupPattern = regexp.MustCompile(`^[A-Za-z0-9:._@/-]{1,128}$`)
	nodeIdPattern          = regexp.MustCompile(`^[A-Za-z0-9]{1,64}$`)
	// A P0 posture, posture:p0_<name>, short enough that its attribute key fits
	// Tailscale's 128 characters.
	posturePattern = regexp.MustCompile(`^posture:p0_[a-z0-9_]{1,118}$`)
	// A 4via6 site ID is a whole number up to 65535 (SITE_ID_PATTERN and MAX_SITE_ID).
	siteIdPattern = regexp.MustCompile(`^\d{1,5}$`)
)

const maxSiteId = 65535

// P0's messages for a missing value (MISSING_TARGET, MISSING_PORTS, MISSING_GROUPS) and
// a site ID it refuses (INVALID_SITE_ID).
const (
	missingTarget = "Enter the destination's target."
	missingPorts  = "Enter at least one port, such as tcp:5432, or * for every port."
	missingGroups = "Enter at least one Kubernetes group, such as system:masters."
	invalidSiteId = "Enter the 4via6 site ID as a whole number from 0 to 65535, or leave it empty."
)

// A comma-separated list as P0 reads it: each entry trimmed, blanks dropped (parseList).
func parseList(value string) []string {
	entries := []string{}
	for _, entry := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(entry); trimmed != "" {
			entries = append(entries, trimmed)
		}
	}
	return entries
}

// Whether every entry of the list value matches pattern.
func eachMatches(pattern *regexp.Regexp) func(string) bool {
	return func(value string) bool {
		for _, entry := range parseList(value) {
			if !pattern.MatchString(entry) {
				return false
			}
		}
		return true
	}
}

func notEmptyList(value string) bool {
	return len(parseList(value)) > 0
}

// The rules for a target: present, then in the form its type takes, if the connector
// has one for it. An IP range and an app connector's name are checked by the connector
// when P0 checks the destination.
func targetRules(form ...rule) []rule {
	return append([]rule{{
		summary: "Missing destination target",
		message: missingTarget,
		test:    func(target string) bool { return target != "" },
	}}, form...)
}

var (
	tagTarget = rule{
		summary: "Invalid destination target",
		message: "The target must be a tag, such as tag:prod.",
		test:    tagPattern.MatchString,
	}
	serviceTarget = rule{
		summary: "Invalid destination target",
		message: "The target must be a service, such as svc:postgres.",
		test:    servicePattern.MatchString,
	}
	nodeIdTarget = rule{
		summary: "Invalid destination target",
		message: "The target must be a Tailscale node id.",
		test:    nodeIdPattern.MatchString,
	}
	postureTarget = rule{
		summary: "Invalid destination target",
		message: "The target must be a P0 posture, such as posture:p0_prod_db.",
		test:    posturePattern.MatchString,
	}

	portsRules = []rule{
		{summary: "Missing ports", message: missingPorts, test: notEmptyList},
		{
			summary: "Invalid port",
			message: "Each port must be a port such as tcp:5432, a range such as tcp:8000-8100, or *.",
			test:    eachMatches(portPattern),
		},
	}
	// P0 refuses a site ID that isn't a whole number in its own words, and the connector
	// one past 65535 in its own. The first says what the second means.
	siteIdRules = []rule{{
		summary: "Invalid 4via6 site ID",
		message: invalidSiteId,
		test: func(siteId string) bool {
			if siteId == "" {
				return true
			}
			number, err := strconv.Atoi(siteId)
			return siteIdPattern.MatchString(siteId) && err == nil && number <= maxSiteId
		},
	}}
	viaRules = []rule{{
		summary: "Invalid router tag",
		message: "The router tag must be a tag, such as tag:prod.",
		test:    func(via string) bool { return via == "" || tagPattern.MatchString(via) },
	}}
	usersRules = []rule{{
		summary: "Invalid SSH user",
		message: "Each SSH user must be an SSH user.",
		test:    eachMatches(sshUserPattern),
	}}
	groupsRules = []rule{
		{summary: "Missing Kubernetes groups", message: missingGroups, test: notEmptyList},
		{
			summary: "Invalid Kubernetes group",
			message: "Each Kubernetes group must be a Kubernetes group.",
			test:    eachMatches(kubernetesGroupPattern),
		},
	}
)

// The fields each type takes, with their rules in the order P0 checks them. A field a
// type has no entry for is one it doesn't take. The proxy's values are checked by its
// attribute's own validator.
var destinationRules = map[string][]fieldRules{
	ipType:           {{name: "target", rules: targetRules()}, {name: "ports", rules: portsRules}, {name: "site_id", rules: siteIdRules}, {name: "via", rules: viaRules}},
	deviceType:       {{name: "target", rules: targetRules(nodeIdTarget)}, {name: "ports", rules: portsRules}},
	tagType:          {{name: "target", rules: targetRules(tagTarget)}, {name: "ports", rules: portsRules}},
	serviceType:      {{name: "target", rules: targetRules(serviceTarget)}, {name: "ports", rules: portsRules}},
	sshType:          {{name: "target", rules: targetRules(tagTarget)}, {name: "users", rules: usersRules}},
	appConnectorType: {{name: "target", rules: targetRules()}},
	capabilityType:   {{name: "target", rules: targetRules(tagTarget)}, {name: "groups", rules: groupsRules}, {name: "proxy"}},
	exitNodeType:     {{name: "target", rules: targetRules(nodeIdTarget)}},
	postureType:      {{name: "target", rules: targetRules(postureTarget)}},
}

// The fields that some types take and others don't, in the order the app's form asks
// for them, and the message for a missing one when a type needs it. emptyIsUnset marks
// a field that a type taking it may leave out, where P0 takes "" as left out and
// fromJson reads it back as null.
var optionalDestinationFields = []struct {
	name         string
	missing      string
	emptyIsUnset bool
}{
	{name: "ports", missing: missingPorts},
	{name: "site_id", emptyIsUnset: true},
	{name: "via", emptyIsUnset: true},
	{name: "users", emptyIsUnset: true},
	{name: "groups", missing: missingGroups},
	{name: "proxy"},
}

// An optional field that P0 holds as "", which the installer's form stores for a field
// an admin typed in and cleared, reads as null, as a field left out does. Otherwise an
// imported destination whose configuration leaves the field out would plan a
// replacement.
func emptyAsNull(value *string) types.String {
	if value == nil || *value == "" {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

// The types that take field, as a sentence lists them.
func typesTaking(field string) string {
	quoted := []string{}
	for _, destinationType := range destinationTypes {
		if slices.ContainsFunc(destinationRules[destinationType], func(f fieldRules) bool { return f.name == field }) {
			quoted = append(quoted, strconv.Quote(destinationType))
		}
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " or " + quoted[len(quoted)-1]
}

// Checking here rather than in Create reports a mistake at plan time, before the
// provider calls P0, in P0's words: the fields each type takes, which of them it needs,
// and the form of each value. A value that Terraform only knows at apply time is
// skipped while planning, and checked when Terraform validates the configuration again
// before it applies. P0 also refuses a destination whose type the tailnet's mode can't
// grant, which this can't check, because it knows the tailnet only by its ID.
func (*TailscaleDestination) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config tailscaleDestinationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	// The type's own validator has already rejected a known type the provider doesn't
	// take.
	if resp.Diagnostics.HasError() || !isSet(config.Type) || !slices.Contains(destinationTypes, config.Type.ValueString()) {
		return
	}

	destinationType := config.Type.ValueString()
	rules := destinationRules[destinationType]
	values := config.fields()
	for _, field := range optionalDestinationFields {
		taken := slices.ContainsFunc(rules, func(f fieldRules) bool { return f.name == field.name })
		switch {
		case !taken && isSet(values[field.name]):
			resp.Diagnostics.AddAttributeError(
				path.Root(field.name),
				"Unexpected "+strings.ReplaceAll(field.name, "_", " "),
				fmt.Sprintf("'%s' may only be set when 'type' is %s.", field.name, typesTaking(field.name)),
			)
		case taken && field.missing != "" && values[field.name].IsNull():
			resp.Diagnostics.AddAttributeError(path.Root(field.name), "Missing "+field.name, field.missing)
		case taken && field.emptyIsUnset && isSet(values[field.name]) && values[field.name].ValueString() == "":
			resp.Diagnostics.AddAttributeError(
				path.Root(field.name),
				"Empty "+strings.ReplaceAll(field.name, "_", " "),
				fmt.Sprintf("Leave '%s' out rather than setting it to \"\": P0 takes an empty value as one left out, and Terraform would read it back as null.", field.name),
			)
		}
	}
	validateFields(path.Empty(), values, rules, &resp.Diagnostics)
}

func (r *TailscaleDestination) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := internal.Configure(&req, resp)
	r.installer = &common.Install{
		Integration:        TailscaleKey,
		Component:          destinationComponent,
		ProviderData:       data,
		GetId:              r.getId,
		GetItemJson:        r.getItemJson,
		FromJson:           r.fromJson,
		ToJson:             r.toJson,
		DescribeCheckError: describeCheckError("destination"),
	}
}

// The key is built from the tailnet and the name, so it's known before P0 has the item.
func (r *TailscaleDestination) getId(data any) *string {
	model, ok := data.(*tailscaleDestinationModel)
	if !ok {
		return nil
	}
	key := model.key()
	return &key
}

// A response with no item is a bad response, not an item to read.
func (r *TailscaleDestination) getItemJson(json any) any {
	inner, ok := json.(*tailscaleDestinationApi)
	if !ok || inner.Item == nil {
		return nil
	}
	return inner.Item
}

// The key names the destination's tailnet, and P0 sets the item's tailnet from it, so
// the tailnet and the name are read from the key.
func (r *TailscaleDestination) fromJson(_ context.Context, diags *diag.Diagnostics, id string, json any) any {
	jsonv, ok := json.(*tailscaleDestinationJson)
	if !ok {
		return nil
	}
	tailnet, name, ok := parseDestinationKey(id)
	if !ok {
		diags.AddError("Bad destination key", fmt.Sprintf("P0 has a Tailscale destination with the key %q, which isn't of the form <Tailnet ID>:<name>", id))
		return nil
	}
	// type is Required, so a missing kind would surface as Terraform core's "provider
	// produced inconsistent result after apply", which tells the reader nothing to do.
	if jsonv.Kind == nil {
		diags.AddError(
			"Bad API response",
			fmt.Sprintf("The Tailscale destination %s carries no type. "+
				"Remove it in the P0 app and add it again, or remove it from Terraform state with `terraform state rm`.", id),
		)
		return nil
	}
	kind := jsonv.Kind
	if !slices.Contains(destinationTypes, kind.Type) {
		diags.AddError(
			"Unsupported Tailscale destination",
			fmt.Sprintf("The Tailscale destination %s has the type %q, which this provider doesn't support. "+
				"Upgrade the provider, manage the destination in the P0 app, or remove it from Terraform state with `terraform state rm`.", id, kind.Type),
		)
		return nil
	}

	// P0 stores the proxy's default for a Kubernetes API destination that leaves it out.
	proxy := types.StringNull()
	if kind.Type == capabilityType {
		proxy = types.StringValue(operatorProxy)
		if kind.Proxy != nil {
			proxy = types.StringValue(kind.Proxy.Type)
		}
	}

	return &tailscaleDestinationModel{
		Id:      types.StringValue(id),
		Tailnet: types.StringValue(tailnet),
		Name:    types.StringValue(name),
		Type:    types.StringValue(kind.Type),
		Target:  types.StringPointerValue(kind.Target),
		Ports:   types.StringPointerValue(kind.Ports),
		SiteId:  emptyAsNull(kind.SiteId),
		Via:     emptyAsNull(kind.Via),
		Users:   emptyAsNull(kind.Users),
		Groups:  types.StringPointerValue(kind.Groups),
		Proxy:   proxy,
		State:   types.StringPointerValue(jsonv.State),
	}
}

// The body of the PUT that stages the item, and of verify and configure: every field,
// which P0 fixes when it creates the item. Staging takes the key's tailnet, but verify
// and configure refuse a tailnet, or any field, that differs from the one P0 saved, so
// each step sends them all. A Kubernetes API destination always sends its proxy, since
// P0 stores the default when it's left out, and then refuses a request without it.
func (r *TailscaleDestination) toJson(data any) any {
	model, ok := data.(*tailscaleDestinationModel)
	if !ok {
		return nil
	}

	kind := kindJson{
		Type:   model.Type.ValueString(),
		Target: model.Target.ValueStringPointer(),
		SiteId: model.SiteId.ValueStringPointer(),
		Ports:  model.Ports.ValueStringPointer(),
		Via:    model.Via.ValueStringPointer(),
		Users:  model.Users.ValueStringPointer(),
		Groups: model.Groups.ValueStringPointer(),
	}
	if kind.Type == capabilityType {
		kind.Proxy = &selectJson{Type: operatorProxy}
		if isSet(model.Proxy) {
			kind.Proxy.Type = model.Proxy.ValueString()
		}
	}
	return &tailscaleDestinationJson{Tailnet: model.Tailnet.ValueStringPointer(), Kind: &kind}
}

// Stages the item, then verifies and configures it, which is when P0 checks the
// destination through the connector. A check that fails leaves the staged item in
// state, tainted, and the next apply replaces it, because P0 refuses a second PUT of a
// destination that's already added.
func (r *TailscaleDestination) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var json tailscaleDestinationApi
	var data tailscaleDestinationModel

	r.installer.EnsureConfig(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &data)
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.Stage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data, r.toJson(&data))
	if resp.Diagnostics.HasError() {
		return
	}

	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &json, &data)
}

func (r *TailscaleDestination) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	r.installer.Read(ctx, &resp.Diagnostics, &resp.State, &tailscaleDestinationApi{}, &tailscaleDestinationModel{})
}

func (*TailscaleDestination) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	planUnfinishedInstall(ctx, req, resp)
}

// Every configurable attribute requires replacement, so the only update Terraform
// plans is the one that finishes an install, which verifies and configures the item as
// Create does.
func (r *TailscaleDestination) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.installer.UpsertFromStage(ctx, &resp.Diagnostics, &req.Plan, &resp.State, &tailscaleDestinationApi{}, &tailscaleDestinationModel{})
}

func (r *TailscaleDestination) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.installer.Delete(ctx, &resp.Diagnostics, &req.State, &tailscaleDestinationModel{})
}

// Imports a destination by its key, `<tailnet>:<name>`. Both halves get the checks that
// the configuration's get, because the read that follows the import puts the key in a
// URL path, where `T1234:db#eu` reads `T1234:db`.
func (r *TailscaleDestination) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tailnet, name, ok := parseDestinationKey(req.ID)
	if !ok {
		resp.Diagnostics.AddError("Invalid destination key", invalidDestinationKey)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tailnet"), tailnet)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}
