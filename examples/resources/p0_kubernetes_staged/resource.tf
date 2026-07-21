# Prerequisite: the cluster's AWS account must already be installed for P0 IAM
# management (e.g. p0_aws / the p0-terraform-install p0_aws_iam_management module).
# P0 resolves the account from the cluster ARN and rejects the install otherwise.

data "aws_eks_cluster" "example" {
  name = "my-eks-cluster"
}

# Staging generates the PKI outputs (ca_bundle, server_cert, server_key) for the
# in-cluster admission controller. See the p0_kubernetes example for the full
# install chain.
resource "p0_kubernetes_staged" "example" {
  id                    = "my-eks-cluster"
  cluster_arn           = data.aws_eks_cluster.example.arn
  cluster_endpoint      = data.aws_eks_cluster.example.endpoint
  certificate_authority = data.aws_eks_cluster.example.certificate_authority[0].data

  # Optional (defaults shown):
  # connectivity_type = "proxy" # or "public" to connect over the public internet
  # auto_mode_enabled = false   # set true if the EKS cluster has Auto Mode enabled
}
