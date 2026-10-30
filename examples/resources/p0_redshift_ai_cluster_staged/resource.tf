# Requires the connector in the cluster's AWS account to be installed (see
# p0_redshift_ai_connector), and P0's AWS IAM management and AWS OIDC for that
# account. The connector's id is the AWS account it runs in.

resource "p0_redshift_ai_cluster_staged" "example" {
  id        = "123456789012/us-west-2/analytics"
  connector = "123456789012"
  type      = "provisioned"
}

# See the p0_redshift_ai_cluster example for the next steps (letting the
# connector's role into the cluster, and creating its database user) that
# complete the install.
