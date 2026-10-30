package installredshiftai

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	installaws "github.com/p0-security/terraform-provider-p0/internal/provider/resources/install/aws"
)

// The checks on where the connector's Lambda runs. The region patterns are the
// ones p0_github_repositories checks its connector's region with.

// An AWS region, such as us-west-2.
var awsRegionRegex = regexp.MustCompile(`^[a-z]+(?:-[a-z]+)+-\d+$`)

// A region outside AWS's commercial partition: GovCloud, China, the ISO regions
// or the European Sovereign Cloud. It lists the other partitions rather than the
// commercial regions, so a new commercial region needs no change here.
var awsOtherPartitionRegionRegex = regexp.MustCompile(`^(?:us-gov-|cn-|[a-z]+-iso[a-z]*-|eusc-)`)

// A cluster's key, `<AWS account ID>/<region>/<name>`, with the patterns that
// the app's `parseClusterId` checks. The name is a provisioned cluster
// identifier or a Serverless workgroup name.
var clusterIdRegex = regexp.MustCompile(`^\d{12}/([a-z]{2}(?:-[a-z]+)+-\d{1,2})/[a-z][a-z0-9-]{0,63}$`)

// Rejects a value that isn't a cluster's key, or whose region is outside AWS's
// commercial partition, where P0 can't invoke the connector.
func clusterIdValidator() validator.String {
	return clusterId{}
}

type clusterId struct{}

func (clusterId) Description(context.Context) string {
	return "a cluster's key, <AWS account ID>/<region>/<cluster identifier or workgroup name>, in a commercial AWS region"
}

func (v clusterId) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (clusterId) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	id := req.ConfigValue.ValueString()
	match := clusterIdRegex.FindStringSubmatch(id)
	switch {
	case match == nil:
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid Redshift cluster ID", fmt.Sprintf("Use <AWS account ID>/<region>/<name>, such as 123456789012/us-west-2/analytics, where the name is the cluster identifier or workgroup name, got: %q", id))
	case awsOtherPartitionRegionRegex.MatchString(match[1]):
		resp.Diagnostics.AddAttributeError(req.Path, "Unsupported AWS region", fmt.Sprintf("P0 supports clusters only in commercial AWS regions, e.g. us-east-1, got: %q", id))
	}
}

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
