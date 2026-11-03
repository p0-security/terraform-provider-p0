package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const bundleAddress = "p0_access_bundle.deploy"

// writeBundle stands in for POST bundles and PUT bundles/:id. Like P0's
// validateBundle, it requires a label and 2 to 15 distinct items, and like
// buildBundlePolicy, it writes the bundle's rule with P0's defaults and
// without nulls.
func (f *fakePolicyP0) writeBundle(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"request body must be a JSON object"}`, http.StatusBadRequest)
		return
	}
	if message := validateFakeBundle(body); message != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": message})
		return
	}

	if !f.beginWrite(w) {
		return
	}
	defer f.endWrite()

	id := r.PathValue("id")
	status := http.StatusOK
	if id == "" {
		f.nextId++
		id = fmt.Sprintf("bundle%d", f.nextId)
		status = http.StatusCreated
	} else if _, ok := f.bundles[id]; !ok {
		notFound(w)
		return
	}

	bundle := map[string]any{"id": id, "createdBy": "terraform", "updatedAt": float64(1700000000000 + f.nextId)}
	for _, key := range []string{"label", "description", "disabled", "requestor", "approval", "items"} {
		if value, ok := body[key]; ok {
			bundle[key] = value
		}
	}
	f.bundles[id] = bundle

	rule := map[string]any{
		"name":      "bundle:" + id,
		"requestor": map[string]any{"type": "any"},
		"approval":  []any{map[string]any{"type": "p0"}},
		"resource": map[string]any{
			"type": "integration", "service": "bundle", "accessType": "any",
			"filters": map[string]any{"bundle": map[string]any{"effect": "keep", "key": "id", "pattern": "^" + id + "$"}},
		},
	}
	if requestor, ok := body["requestor"]; ok {
		rule["requestor"] = requestor
	}
	if approval, ok := body["approval"].([]any); ok && len(approval) > 0 {
		rule["approval"] = approval
	}
	if disabled, ok := body["disabled"]; ok {
		rule["disabled"] = disabled
	}
	f.policies["bundle:"+id], _ = json.Marshal(withoutNulls(rule))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"bundle": bundle})
}

func validateFakeBundle(body map[string]any) string {
	if label, _ := body["label"].(string); strings.TrimSpace(label) == "" {
		return "label is required"
	}
	items, ok := body["items"].([]any)
	if !ok {
		return "items must be an array"
	}
	if len(items) < 2 {
		return "A bundle must have at least 2 items"
	}
	if len(items) > 15 {
		return "A bundle must have at most 15 items"
	}
	seen := map[string]bool{}
	for _, item := range items {
		key, _ := json.Marshal(item)
		if seen[string(key)] {
			return "Duplicate items are not allowed"
		}
		seen[string(key)] = true
	}
	return ""
}

func (f *fakePolicyP0) readBundle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	bundle, ok := f.bundles[r.PathValue("id")]
	if !ok {
		notFound(w)
		return
	}
	writeJson(w, map[string]any{"bundle": bundle})
}

func (f *fakePolicyP0) deleteBundle(w http.ResponseWriter, r *http.Request) {
	if !f.beginWrite(w) {
		return
	}
	defer f.endWrite()

	id := r.PathValue("id")
	if _, ok := f.bundles[id]; !ok {
		notFound(w)
		return
	}
	delete(f.bundles, id)
	delete(f.policies, "bundle:"+id)
	w.WriteHeader(http.StatusNoContent)
}

// Changes or deletes a stored rule behind Terraform's back, to simulate drift.
// A nil change deletes it.
func (f *fakePolicyP0) updatePolicy(name string, change func(rule map[string]any)) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if change == nil {
		delete(f.policies, name)
		return
	}
	var rule map[string]any
	_ = json.Unmarshal(f.policies[name], &rule)
	change(rule)
	f.policies[name], _ = json.Marshal(rule)
}

func (f *fakePolicyP0) policy(name string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()

	stored, ok := f.policies[name]
	if !ok {
		return nil
	}
	var rule map[string]any
	_ = json.Unmarshal(stored, &rule)
	return rule
}

// The only bundle in the fake, as Terraform tests create one at a time.
func (f *fakePolicyP0) bundleId() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	for id := range f.bundles {
		return id
	}
	return ""
}

// withoutNulls drops null fields, as P0 does when it stores a rule.
func withoutNulls(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		stripped := map[string]any{}
		for key, field := range typed {
			if field != nil {
				stripped[key] = withoutNulls(field)
			}
		}
		return stripped
	case []any:
		stripped := make([]any, len(typed))
		for i, element := range typed {
			stripped[i] = withoutNulls(element)
		}
		return stripped
	}
	return value
}

func bundleProviderConfig(f *fakePolicyP0, previewEnabled bool) string {
	return fmt.Sprintf(`
provider "p0" {
  org                      = %q
  host                     = %q
  api_token                = "test-token"
  enable_preview_resources = %t
}
`, fakeOrg, f.server.URL, previewEnabled)
}

// A bundle with a group approval, a gcloud role and an AWS permission set.
// args is spliced in as the gcloud item's args.
func bundleConfig(f *fakePolicyP0, label, gcloudArgs string) string {
	return bundleProviderConfig(f, true) + fmt.Sprintf(`
resource "p0_access_bundle" "deploy" {
  label       = %q
  description = "Deploy with Terraform"
  approval = [{
    type    = "group"
    effect  = "keep"
    groups  = [{ directory = "workspace", id = "00xvir7l2dtpb3m", label = "approvers@example.com" }]
    options = { require_reason = true }
  }]
  items = [
    { type = "gcloud", access = "role", args = %s },
    { type = "aws", access = "permission-set", args = jsonencode({ account = "123456789012", name = "arn:aws:sso:::permissionSet/ssoins-1/ps-1" }) },
  ]
}
`, label, gcloudArgs)
}

const gcloudArgs = `jsonencode({ project = "prod", name = ["roles/editor"] })`

// The same args, with the keys out of order and extra whitespace. The fake
// returns them sorted and compact, as encoding/json does.
const gcloudArgsReordered = `"{ \"name\": [\"roles/editor\"],  \"project\": \"prod\" }"`

// Checks that the bundle's rule exists and has the bundle's one group approval.
func checkBundleRule(f *fakePolicyP0) resource.TestCheckFunc {
	return func(*terraform.State) error {
		name := "bundle:" + f.bundleId()
		rule := f.policy(name)
		if rule == nil {
			return fmt.Errorf("P0 has no rule %q", name)
		}
		approval, _ := rule["approval"].([]any)
		if len(approval) != 1 {
			return fmt.Errorf("rule %q has approval %v, want the bundle's group approval", name, approval)
		}
		if first, _ := approval[0].(map[string]any); first["type"] != "group" {
			return fmt.Errorf("rule %q has approval %v, want the bundle's group approval", name, approval)
		}
		return nil
	}
}

// TestAccessBundleLifecycle creates, updates, imports and destroys a bundle.
// The first step's args differ in key order and whitespace from what the fake
// returns, so its empty follow-up plan shows they don't produce a diff.
func TestAccessBundleLifecycle(t *testing.T) {
	f := newFakePolicyP0(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.bundles) > 0 || len(f.policies) > 0 {
				return fmt.Errorf("P0 still has %d bundles and %d rules", len(f.bundles), len(f.policies))
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: bundleConfig(f, "Terraform deploy", gcloudArgsReordered),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(bundleAddress, "id", "bundle1"),
					resource.TestCheckResourceAttr(bundleAddress, "items.0.args", `{ "name": ["roles/editor"],  "project": "prod" }`),
					resource.TestCheckResourceAttr(bundleAddress, "policy_name", "bundle:bundle1"),
					resource.TestCheckResourceAttr(bundleAddress, "created_by", "terraform"),
					resource.TestCheckResourceAttr(bundleAddress, "items.#", "2"),
					resource.TestCheckResourceAttr(bundleAddress, "items.1.access", "permission-set"),
					checkBundleRule(f),
				),
			},
			{
				Config: bundleConfig(f, "Deploy", gcloudArgs),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(bundleAddress, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(bundleAddress, "label", "Deploy"),
					resource.TestCheckResourceAttr(bundleAddress, "id", "bundle1"),
				),
			},
			{
				ResourceName:      bundleAddress,
				ImportState:       true,
				ImportStateId:     "bundle1",
				ImportStateVerify: true,
			},
			{
				Config: bundleConfig(f, "Deploy", gcloudArgs),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// TestAccessBundleValidation checks the item limits at plan time, and that a
// refusal from P0's validateBundle reaches the user.
func TestAccessBundleValidation(t *testing.T) {
	f := newFakePolicyP0(t)
	items := func(n int) string {
		var config strings.Builder
		for i := range n {
			fmt.Fprintf(&config, "    { type = \"gcloud\", access = \"role\", args = jsonencode({ project = \"p%d\", name = [\"roles/viewer\"] }) },\n", i)
		}
		return config.String()
	}
	config := func(items string) string {
		return bundleProviderConfig(f, true) + fmt.Sprintf(`
resource "p0_access_bundle" "deploy" {
  label = "Deploy"
  items = [
%s  ]
}
`, items)
	}
	duplicate := `    { type = "gcloud", access = "role", args = jsonencode({ project = "p", name = ["roles/viewer"] }) },
`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config(items(1)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Attribute items list must contain at least 2 elements and at most 15`),
			},
			{
				Config:      config(items(16)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Attribute items list must contain at least 2 elements and at most 15`),
			},
			{
				Config:      config(duplicate + duplicate),
				ExpectError: regexp.MustCompile(`Unable to create access bundle:\s+400 Bad Request: Duplicate items are not\s+allowed`),
			},
		},
	})
}

// TestAccessBundleRequiresPreview checks that planning and importing a
// bundle fail while enable_preview_resources is off.
func TestAccessBundleRequiresPreview(t *testing.T) {
	f := newFakePolicyP0(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      strings.Replace(bundleConfig(f, "Deploy", gcloudArgs), "enable_preview_resources = true", "enable_preview_resources = false", 1),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`p0_access_bundle is a preview resource`),
			},
			{
				Config: strings.Replace(bundleConfig(f, "Deploy", gcloudArgs), "enable_preview_resources = true", "enable_preview_resources = false", 1) + `
import {
  to = p0_access_bundle.deploy
  id = "bundle1"
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`p0_access_bundle is a preview resource`),
			},
		},
	})
}

// TestAccessBundleDrift checks that a deleted or changed bundle rule plans an
// update that restores it, and that a bundle deleted outside Terraform is
// created again.
func TestAccessBundleDrift(t *testing.T) {
	f := newFakePolicyP0(t)
	config := bundleConfig(f, "Deploy", gcloudArgs)
	expectUpdate := resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(bundleAddress, plancheck.ResourceActionUpdate)},
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  checkBundleRule(f),
			},
			{
				PreConfig:        func() { f.updatePolicy("bundle:"+f.bundleId(), nil) },
				Config:           config,
				ConfigPlanChecks: expectUpdate,
				Check:            checkBundleRule(f),
			},
			{
				PreConfig: func() {
					f.updatePolicy("bundle:"+f.bundleId(), func(rule map[string]any) {
						rule["approval"] = []any{map[string]any{"type": "persistent"}}
					})
				},
				Config:           config,
				ConfigPlanChecks: expectUpdate,
				Check:            checkBundleRule(f),
			},
			{
				PreConfig: func() {
					f.mu.Lock()
					defer f.mu.Unlock()
					clear(f.bundles)
					clear(f.policies)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(bundleAddress, plancheck.ResourceActionCreate)},
				},
				Check: resource.TestCheckResourceAttr(bundleAddress, "id", "bundle2"),
			},
		},
	})
}

// TestPolicyAndBundleWritesAreSerialized applies five policies and five
// bundles at Terraform's default parallelism of 10, and checks that their
// writes never overlapped and that every rule is in place.
func TestPolicyAndBundleWritesAreSerialized(t *testing.T) {
	f := newFakePolicyP0(t)
	var config strings.Builder
	config.WriteString(bundleProviderConfig(f, true))
	for i := range 5 {
		fmt.Fprintf(&config, `
resource "p0_access_policy" "policy_%d" {
  name      = "policy-%d"
  requestor = { type = "any" }
  resource  = { type = "any" }
  approval  = [{ type = "p0" }]
}

resource "p0_access_bundle" "bundle_%d" {
  label = "Bundle %d"
  items = [
    { type = "gcloud", access = "role", args = jsonencode({ project = "p%d", name = ["roles/viewer"] }) },
    { type = "gcloud", access = "role", args = jsonencode({ project = "p%d", name = ["roles/editor"] }) },
  ]
}
`, i, i, i, i, i, i)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkNoOverlappingPolicyWrites(f),
		Steps: []resource.TestStep{
			{
				Config: config.String(),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkNoOverlappingPolicyWrites(f),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						for i := range 5 {
							if _, ok := f.policies[fmt.Sprintf("policy-%d", i)]; !ok {
								return fmt.Errorf("P0 has no rule policy-%d", i)
							}
						}
						for id := range f.bundles {
							if _, ok := f.policies["bundle:"+id]; !ok {
								return fmt.Errorf("P0 has no rule bundle:%s", id)
							}
						}
						if len(f.bundles) != 5 || len(f.policies) != 10 {
							return fmt.Errorf("P0 has %d bundles and %d rules, want 5 and 10", len(f.bundles), len(f.policies))
						}
						return nil
					},
				),
			},
		},
	})
}
