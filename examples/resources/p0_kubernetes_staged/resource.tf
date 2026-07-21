# Prerequisite: the EKS cluster's AWS account must already be installed for P0
# IAM management (for example via the p0_aws resource / the p0-terraform-install
# p0_aws_iam_management module) before this integration can be applied. The P0
# backend resolves the AWS account from the cluster ARN and rejects the install
# if that account is not yet installed for IAM management.

# Reference the EKS cluster P0 will manage access to. This data source
# supplies the cluster ARN, API endpoint, and certificate authority.
data "aws_eks_cluster" "example" {
  name = "my-eks-cluster"
}

# Stage the K8s integration. Staging generates the PKI values
# (ca_bundle, server_cert, server_key) used to configure the in-cluster
# P0 admission controller. See the p0_kubernetes example for the full
# installation chain that completes the install.
resource "p0_kubernetes_staged" "example" {
  id                    = "my-eks-cluster"
  cluster_arn           = data.aws_eks_cluster.example.arn
  cluster_endpoint      = data.aws_eks_cluster.example.endpoint
  certificate_authority = data.aws_eks_cluster.example.certificate_authority[0].data

  # Optional attributes (defaults shown):
  # connectivity_type = "proxy" # connect via P0's proxy service; use "public" to connect over the public internet
  # hosting_type      = "aws"
  # auto_mode_enabled = false   # set to true if the EKS cluster has Auto Mode enabled
}
