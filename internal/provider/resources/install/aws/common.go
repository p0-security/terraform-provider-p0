package installaws

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	installresources "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install"
)

const Aws = "aws"

// All installable AWS components.
var Components = []string{installresources.IamWrite, installresources.Inventory}

var AwsAccountIdRegex = regexp.MustCompile(`^\d{12}$`)
var AwsPartitionRegex = regexp.MustCompile(`^(aws|aws-us-gov)$`)
var AwsIdpPattern = regexp.MustCompile(`^[\w.-/]+$`)
var OktaAppIdRegex = regexp.MustCompile(`^0o\w+$`)

// An AWS region, such as us-west-2.
var AwsRegionRegex = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+-\d+$`)

// A region outside AWS's commercial partition: GovCloud, China, the ISO regions or the
// European Sovereign Cloud. It lists the other partitions rather than the commercial
// regions, so a new commercial region needs no change here.
var AwsOtherPartitionRegionRegex = regexp.MustCompile(`^(?:us-gov-|cn-|[a-z]+-iso[a-z]*-|eusc-)`)

const AwsLabelMarkdownDescription = "The AWS account's alias (if available)"

func AwsAccountIdValidator() validator.String {
	return stringvalidator.RegexMatches(AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits")
}

// Rejects a value that isn't an AWS region in the commercial partition, with message. A
// connector that P0 invokes by an `arn:aws:` ARN can't run anywhere else.
func CommercialRegionValidator(message string) validator.String {
	return commercialRegionValidator{message: message}
}

type commercialRegionValidator struct {
	message string
}

func (v commercialRegionValidator) Description(context.Context) string {
	return v.message
}

func (v commercialRegionValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v commercialRegionValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	region := req.ConfigValue.ValueString()
	if !AwsRegionRegex.MatchString(region) || AwsOtherPartitionRegionRegex.MatchString(region) {
		resp.Diagnostics.AddAttributeError(req.Path, "Unsupported AWS region", fmt.Sprintf("%s, got: %s", v.message, region))
	}
}
