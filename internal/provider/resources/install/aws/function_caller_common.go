package installaws

import (
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

const FunctionCaller = "function-caller"

// A Lambda function's ARN, with an optional version or alias. P0 takes the account
// and partition from it.
var LambdaArnRegex = regexp.MustCompile(`^arn:(aws|aws-us-gov):lambda:[a-z0-9-]+:\d{12}:function:[A-Za-z0-9_-]{1,64}(:[A-Za-z0-9_$-]{1,128})?$`)

// LambdaArnValidator checks that a value is a Lambda function's ARN.
var LambdaArnValidator = stringvalidator.RegexMatches(LambdaArnRegex, "Enter the function's ARN: arn:<partition>:lambda:<region>:<account ID>:function:<name>")

// P0 keys a function caller by its Lambda's ARN, so a change replaces the install.
func functionCallerIdAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: description,
		Validators:          []validator.String{LambdaArnValidator},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

// P0's check reads the inline policy of its function-caller role in the function's
// account. A failure usually means the role or its policy isn't in place yet, which
// P0's message says.
func describeFunctionCallerCheckError(id string, err error) (string, string) {
	return "AWS function caller install check failed",
		fmt.Sprintf("P0 rejected the install check for the Lambda function %q:\n\n%s", id, err)
}
