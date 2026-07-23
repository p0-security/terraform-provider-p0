# Prerequisite: the cluster's AWS account must already be installed for P0 IAM
# management (e.g. p0_aws / the p0-terraform-install p0_aws_iam_management module).
# P0 resolves the account from the cluster ARN and rejects the install otherwise.

data "aws_eks_cluster" "example" {
  name = "my-eks-cluster"
}

# P0 authenticates to the cluster with the token in the p0-service-account-secret
# Secret (p0-security namespace), created when the in-cluster components deploy below:
#   kubectl get secret p0-service-account-secret -o json -n p0-security | jq -r '.data.token' | base64 -d
data "kubernetes_secret" "p0" {
  metadata {
    name      = "p0-service-account-secret"
    namespace = "p0-security"
  }
}

# Staging generates the PKI outputs (ca_bundle / server_cert / server_key) that
# configure the in-cluster admission controller and braekhus proxy.
resource "p0_kubernetes_staged" "example" {
  id                    = "my-eks-cluster"
  cluster_arn           = data.aws_eks_cluster.example.arn
  cluster_endpoint      = data.aws_eks_cluster.example.endpoint
  certificate_authority = data.aws_eks_cluster.example.certificate_authority[0].data
}

# Deploy the in-cluster components with the staged PKI, then read the proxy's
# public JWK once its pod is running:
#   kubectl exec -it deploy/p0-braekhus-proxy -n p0-security -c braekhus -- cat /p0-files/jwk.public.json | jq -Mr
# public_jwk is always sent by the provider even for connectivity_type = "public"; P0 only uses it for proxy connectivity.
variable "braekhus_public_jwk" {
  type        = string
  description = "Public JWK of the in-cluster braekhus proxy service"
}

# Completes the install; verification only succeeds after the staged resource
# exists and the in-cluster components are running (hence depends_on).
resource "p0_kubernetes" "example" {
  id                    = p0_kubernetes_staged.example.id
  token                 = data.kubernetes_secret.p0.data["token"]
  public_jwk            = var.braekhus_public_jwk
  cluster_arn           = data.aws_eks_cluster.example.arn
  cluster_endpoint      = data.aws_eks_cluster.example.endpoint
  certificate_authority = data.aws_eks_cluster.example.certificate_authority[0].data

  depends_on = [p0_kubernetes_staged.example]
}
