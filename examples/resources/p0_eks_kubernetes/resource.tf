resource "p0_kubernetes" "tf-test-cluster" {
  id                    = "my-eks-cluster"
  token                 = "ABIGBASE64TOKENSTRING"
  public_jwk            = "{\"kty\":\"XYZ\",\"x\":\"7KbVnH-WZt9AhV5\",\"y\":\"d4RrlJPrWYkTAHV1Qww5-gV\",\"crv\":\"P-404\"}"
  cluster_arn           = "arn:aws:eks:us-west-2:123456789101:cluster/my-eks-cluster"
  cluster_endpoint      = "https://ABC123XYC211242IAM.gr7.us-west-2.eks.amazonaws.com"
  certificate_authority = "ABIGBASE64CERTSTRING"
}