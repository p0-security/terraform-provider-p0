package installdatabricks

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
)

// The checks on where the connector's Lambda runs. The region patterns and their
// messages are the ones p0_github_repositories checks its connector's region with.

// An AWS region, such as us-west-2.
var awsRegionRegex = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+-\d+$`)

// A region outside AWS's commercial partition: GovCloud, China, the ISO regions
// or the European Sovereign Cloud. It lists the other partitions rather than the
// commercial regions, so a new commercial region needs no change here.
var awsOtherPartitionRegionRegex = regexp.MustCompile(`^(?:us-gov-|cn-|[a-z]+-iso[a-z]*-|eusc-)`)

// Rejects a value that isn't an AWS account ID, with the message of the AWS
// install resources.
func awsAccountIdValidator() validator.String {
	return stringvalidator.RegexMatches(installaws.AwsAccountIdRegex, "AWS account IDs should consist of 12 numeric digits")
}

// Rejects a value that isn't an AWS region, and, with message, a region outside
// AWS's commercial partition. P0 invokes the connector by an `arn:aws:` ARN, so
// it can't run anywhere else.
func commercialRegionValidator(message string) validator.String {
	return commercialRegion{message: message}
}

type commercialRegion struct {
	message string
}

func (v commercialRegion) Description(context.Context) string {
	return v.message
}

func (v commercialRegion) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v commercialRegion) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	region := req.ConfigValue.ValueString()
	// Each message quotes the value, so that whitespace around it shows.
	switch {
	case !awsRegionRegex.MatchString(region):
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid AWS region", fmt.Sprintf("Enter an AWS region, such as us-west-2, got: %q", region))
	case awsOtherPartitionRegionRegex.MatchString(region):
		resp.Diagnostics.AddAttributeError(req.Path, "Unsupported AWS region", fmt.Sprintf("%s, got: %q", v.message, region))
	}
}
