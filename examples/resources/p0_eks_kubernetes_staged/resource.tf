resource "p0_kubernetes_staged" "tf-test-cluster" {
  id                    = "my-eks-cluster"
  cluster_arn           = "arn:aws:eks:us-west-2:123456789101:cluster/my-eks-cluster"
  cluster_endpoint      = "https://ABC123XYC211242IAM.gr7.us-west-2.eks.amazonaws.com"
  certificate_authority = "ABIGBASE64CERTSTRING"
}