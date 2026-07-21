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

# P0 authenticates to the cluster API with the long-lived service-account token
# of the p0-service-account-secret Secret in the p0-security namespace. This
# Secret is created when the in-cluster P0 components (the p0-service-account
# ServiceAccount and its token Secret) are deployed in step 2. You can also read
# the token directly with:
#   kubectl get secret p0-service-account-secret -o json -n p0-security | jq -r '.data.token' | base64 -d
data "kubernetes_secret" "p0" {
  metadata {
    name      = "p0-service-account-secret"
    namespace = "p0-security"
  }
}

# Step 1: stage the K8s integration. Staging generates the PKI values
# (p0_kubernetes_staged.example.ca_bundle / .server_cert / .server_key)
# that are used to configure the in-cluster P0 admission controller and
# braekhus proxy.
resource "p0_kubernetes_staged" "example" {
  id                    = "my-eks-cluster"
  cluster_arn           = data.aws_eks_cluster.example.arn
  cluster_endpoint      = data.aws_eks_cluster.example.endpoint
  certificate_authority = data.aws_eks_cluster.example.certificate_authority[0].data
}

# Step 2: deploy the in-cluster P0 components (admission controller and, for
# the default connectivity_type = "proxy", the braekhus proxy) using the
# staged PKI outputs above. Once the proxy pod is running, read its public JWK
# from inside the pod and supply it here:
#   kubectl exec -it deploy/p0-braekhus-proxy -n p0-security -c braekhus -- cat /p0-files/jwk.public.json | jq -Mr
# Note: public_jwk is currently required by the provider even when
# connectivity_type = "public" (the provider always sends it); the P0 backend
# only uses it for proxy connectivity.
variable "braekhus_public_jwk" {
  type        = string
  description = "Public JWK of the in-cluster braekhus proxy service"
}

# Step 3: complete the installation. This resource verifies the integration
# and only validates successfully after the staged resource exists and the
# in-cluster P0 components are running.
resource "p0_kubernetes" "example" {
  id                    = p0_kubernetes_staged.example.id
  token                 = data.kubernetes_secret.p0.data["token"]
  public_jwk            = var.braekhus_public_jwk
  cluster_arn           = data.aws_eks_cluster.example.arn
  cluster_endpoint      = data.aws_eks_cluster.example.endpoint
  certificate_authority = data.aws_eks_cluster.example.certificate_authority[0].data

  depends_on = [p0_kubernetes_staged.example]
}
