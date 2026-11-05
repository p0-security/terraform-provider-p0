package provider

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	notifierLambdaArn      = "arn:aws:lambda:us-west-2:123456789012:function:p0-notifier"
	notifierOtherLambdaArn = "arn:aws:lambda:us-west-2:123456789012:function:p0-notifier:live"
	notifierId             = "stage-notifier"
	functionCallerStaged   = "p0_aws_function_caller_staged.example"
	functionCallerFinal    = "p0_aws_function_caller.example"
	lambdaNotifier         = "p0_aws_lambda_notifier.example"
)

type lambdaNotifierFake struct {
	*fakeP0
	// When set, verifying a function caller fails the install check with this message.
	failedCheck string
	// The states that each write to a notifier moved it to, in order.
	notifierSteps []string
}

// Adds what P0's AWS installer does for a function caller: the account and partition
// from the ARN when it is staged, and the role to create as the item's metadata.
func newLambdaNotifierFake(t *testing.T) *lambdaNotifierFake {
	f := &lambdaNotifierFake{fakeP0: newFakeP0(t)}
	f.normalize = func(key fakeItemKey, item map[string]any) {
		if key.integration == "aws" && key.component == "function-caller" {
			item["accountId"] = "123456789012"
			item["awsPartition"] = "aws"
		}
	}
	f.metadata = func(key fakeItemKey, _ map[string]any) map[string]any {
		if key.integration != "aws" || key.component != "function-caller" {
			return nil
		}
		return map[string]any{
			"roleName":         "P0RoleIamFunctionCaller",
			"inlinePolicyName": "P0RoleIamFunctionCallerPolicy",
			"inlinePolicy":     `{"Version":"2012-10-17","Statement":[{"Action":["lambda:InvokeFunction"],"Resource":["` + key.id + `"]}]}`,
			"serviceAccountId": "101234567890123456789",
			"trustPolicy":      `{"Version":"2012-10-17"}`,
		}
	}
	f.refuse = func(step fakeStep, _ map[fakeItemKey]map[string]any) (int, string) {
		switch {
		case step.key.integration == "aws-lambda-notifier":
			f.notifierSteps = append(f.notifierSteps, step.nextState)
		case step.key.component == "function-caller" && step.nextState == "configure" && f.failedCheck != "":
			return http.StatusBadRequest, f.failedCheck
		}
		return 0, ""
	}
	return f
}

func (f *lambdaNotifierFake) notifier() map[string]any {
	return f.item("aws-lambda-notifier", "notify", notifierId)
}

func (f *lambdaNotifierFake) functionCaller() map[string]any {
	return f.item("aws", "function-caller", notifierLambdaArn)
}

func (f *lambdaNotifierFake) notifierStepCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.notifierSteps)
}

func (f *lambdaNotifierFake) notifierStepsSince(from int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.notifierSteps[from:])
}

func functionCallerStagedConfig(f *lambdaNotifierFake) string {
	return providerConfig(f.fakeP0) + fmt.Sprintf(`
resource "p0_aws_function_caller_staged" "example" {
  id = %q
}
`, notifierLambdaArn)
}

func functionCallerConfig(f *lambdaNotifierFake) string {
	return functionCallerStagedConfig(f) + `
resource "p0_aws_function_caller" "example" {
  id = p0_aws_function_caller_staged.example.id
}
`
}

// The function caller and a notifier that invokes function, which is the function
// caller's id unless it is set.
func lambdaNotifierConfig(f *lambdaNotifierFake, function string) string {
	functionCaller := "p0_aws_function_caller.example.id"
	if function != "" {
		functionCaller = fmt.Sprintf("%q", function)
	}
	return functionCallerConfig(f) + fmt.Sprintf(`
resource "p0_aws_lambda_notifier" "example" {
  id              = %q
  function_caller = %s
}
`, notifierId, functionCaller)
}

func checkLambdaNotifierDestroyed(f *lambdaNotifierFake) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if item := f.notifier(); item != nil {
			return fmt.Errorf("the notifier was not removed from P0: %v", item)
		}
		if item := f.functionCaller(); item != nil {
			return fmt.Errorf("the function caller was not removed from P0: %v", item)
		}
		return nil
	}
}

func TestAwsLambdaNotifier(t *testing.T) {
	f := newLambdaNotifierFake(t)
	var installed int

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLambdaNotifierDestroyed(f),
		Steps: []resource.TestStep{
			// Staging alone gets the role to create.
			{
				Config: functionCallerStagedConfig(f),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(functionCallerStaged, "state", "stage"),
					resource.TestCheckResourceAttr(functionCallerStaged, "account_id", "123456789012"),
					resource.TestCheckResourceAttr(functionCallerStaged, "partition", "aws"),
					resource.TestCheckResourceAttr(functionCallerStaged, "service_account_id", "101234567890123456789"),
					resource.TestCheckResourceAttr(functionCallerStaged, "role.name", "P0RoleIamFunctionCaller"),
					resource.TestCheckResourceAttr(functionCallerStaged, "role.inline_policy_name", "P0RoleIamFunctionCallerPolicy"),
					resource.TestCheckResourceAttr(functionCallerStaged, "role.trust_policy", `{"Version":"2012-10-17"}`),
					resource.TestMatchResourceAttr(functionCallerStaged, "role.inline_policy", harborError(notifierLambdaArn)),
				),
			},
			// The framework re-plans after the step and fails on a non-empty plan, so this
			// also checks that the fields P0 assigns leave no diff.
			{
				Config: lambdaNotifierConfig(f, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(functionCallerFinal, "state", "installed"),
					resource.TestCheckResourceAttr(functionCallerFinal, "label", notifierLambdaArn),
					resource.TestCheckResourceAttr(lambdaNotifier, "state", "installed"),
					resource.TestCheckResourceAttr(lambdaNotifier, "function_caller", notifierLambdaArn),
					func(*terraform.State) error {
						if item := f.notifier(); item["functionCaller"] != "aws:"+notifierLambdaArn || item["state"] != "installed" {
							return fmt.Errorf("P0 notifier does not name the function caller: %v", item)
						}
						if got, want := f.notifierStepsSince(0), []string{"stage", "configure", "installed"}; !slices.Equal(got, want) {
							return fmt.Errorf("the notifier took the steps %v, want %v", got, want)
						}
						installed = f.notifierStepCount()
						return nil
					},
				),
			},
			{
				ResourceName:      functionCallerStaged,
				ImportState:       true,
				ImportStateId:     notifierLambdaArn,
				ImportStateVerify: true,
				// The staged resource's state is from before the install was completed.
				ImportStateVerifyIgnore: []string{"state"},
			},
			{
				ResourceName:      functionCallerFinal,
				ImportState:       true,
				ImportStateId:     notifierLambdaArn,
				ImportStateVerify: true,
			},
			{
				ResourceName:      lambdaNotifier,
				ImportState:       true,
				ImportStateId:     notifierId,
				ImportStateVerify: true,
			},
			// P0 takes a new function on the configure step, so the notifier changes in
			// place, without being staged again.
			{
				Config: lambdaNotifierConfig(f, notifierOtherLambdaArn),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(functionCallerFinal, plancheck.ResourceActionNoop),
						plancheck.ExpectResourceAction(lambdaNotifier, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(lambdaNotifier, "function_caller", notifierOtherLambdaArn),
					func(*terraform.State) error {
						if got, want := f.notifierStepsSince(installed), []string{"installed"}; !slices.Equal(got, want) {
							return fmt.Errorf("the update took the steps %v, want %v", got, want)
						}
						if item := f.notifier(); item["functionCaller"] != "aws:"+notifierOtherLambdaArn || item["state"] != "installed" {
							return fmt.Errorf("P0 notifier does not name the new function: %v", item)
						}
						return nil
					},
				),
			},
			// A function changed outside Terraform shows as a diff.
			{
				PreConfig: func() {
					f.update("aws-lambda-notifier", "notify", notifierId, func(item map[string]any) {
						item["functionCaller"] = "aws:" + notifierLambdaArn
					})
				},
				Config:             lambdaNotifierConfig(f, notifierOtherLambdaArn),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// Destroying only the final function caller returns its item to "stage", so the
// staged resource still owns it.
func TestAwsFunctionCallerRollback(t *testing.T) {
	f := newLambdaNotifierFake(t)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLambdaNotifierDestroyed(f),
		Steps: []resource.TestStep{
			{Config: functionCallerConfig(f)},
			{
				Config: functionCallerStagedConfig(f),
				Check: func(*terraform.State) error {
					if item := f.functionCaller(); item == nil || item["state"] != "stage" {
						return fmt.Errorf("P0 item was not returned to stage: %v", item)
					}
					return nil
				},
			},
		},
	})
}

// A check that fails, as when the role has no policy yet, fails the apply with P0's
// message. The next apply checks again.
func TestAwsFunctionCallerFailedCheck(t *testing.T) {
	f := newLambdaNotifierFake(t)
	f.failedCheck = "Failed to verify permissions for ARN " + notifierLambdaArn + ": role P0RoleIamFunctionCaller has no inline policy"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkLambdaNotifierDestroyed(f),
		Steps: []resource.TestStep{
			{
				Config:      lambdaNotifierConfig(f, ""),
				ExpectError: harborError("AWS function caller install check failed", notifierLambdaArn, f.failedCheck),
			},
			{
				PreConfig: func() { f.failedCheck = "" },
				Config:    lambdaNotifierConfig(f, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(functionCallerFinal, "state", "installed"),
					resource.TestCheckResourceAttr(lambdaNotifier, "state", "installed"),
				),
			},
		},
	})
}

func TestAwsLambdaNotifierValidation(t *testing.T) {
	for _, tc := range []struct {
		name, config, error string
	}{
		{"function caller without an arn", `
resource "p0_aws_function_caller_staged" "example" {
  id = "p0-notifier"
}
`, "Enter the function's ARN"},
		{"function caller with another service's arn", `
resource "p0_aws_function_caller" "example" {
  id = "arn:aws:sqs:us-west-2:123456789012:p0-notifier"
}
`, "Enter the function's ARN"},
		{"notifier with a function name", `
resource "p0_aws_lambda_notifier" "example" {
  id              = "stage-notifier"
  function_caller = "p0-notifier"
}
`, "Enter the function's ARN"},
		{"notifier with a prefixed arn", fmt.Sprintf(`
resource "p0_aws_lambda_notifier" "example" {
  id              = "stage-notifier"
  function_caller = "aws:%s"
}
`, notifierLambdaArn), "Enter the function's ARN"},
		{"notifier id with a slash", fmt.Sprintf(`
resource "p0_aws_lambda_notifier" "example" {
  id              = "stage/notifier"
  function_caller = %q
}
`, notifierLambdaArn), "can't contain a /"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLambdaNotifierFake(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      providerConfig(f.fakeP0) + tc.config,
						PlanOnly:    true,
						ExpectError: harborError(tc.error),
					},
				},
			})
		})
	}
}
