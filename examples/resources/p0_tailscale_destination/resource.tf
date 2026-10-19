# Destinations on the direct-write tailnet that the p0_tailscale_tailnet example
# adds: one of each common type. Requestors ask for each by its name, as its own
# access. Changing any attribute replaces the destination; open grants keep what
# they were made with.

# Postgres on a subnet behind the router tagged tag:prod-router.
resource "p0_tailscale_destination" "prod_db" {
  tailnet = p0_tailscale_tailnet.example.id
  name    = "prod-db"
  type    = "ip"
  target  = "10.0.4.0/24"
  ports   = "tcp:5432"
  via     = "tag:prod-router"
}

# HTTPS on every device tagged tag:prod-web.
resource "p0_tailscale_destination" "prod_web" {
  tailnet = p0_tailscale_tailnet.example.id
  name    = "prod-web"
  type    = "tag"
  target  = "tag:prod-web"
  ports   = "tcp:443"
}

# A Tailscale Service.
resource "p0_tailscale_destination" "postgres" {
  tailnet = p0_tailscale_tailnet.example.id
  name    = "postgres"
  type    = "service"
  target  = "svc:postgres"
  ports   = "tcp:5432"
}

# Tailscale SSH to every device tagged tag:prod, as any user but root, or as
# root.
resource "p0_tailscale_destination" "prod_ssh" {
  tailnet = p0_tailscale_tailnet.example.id
  name    = "prod-ssh"
  type    = "ssh"
  target  = "tag:prod"
  users   = "autogroup:nonroot,root"
}

# The Kubernetes API through the Tailscale Kubernetes operator, as cluster
# admins. For an API server ProxyGroup, set proxy = "proxy-group".
resource "p0_tailscale_destination" "prod_k8s" {
  tailnet = p0_tailscale_tailnet.example.id
  name    = "prod-k8s"
  type    = "capability"
  target  = "tag:k8s-operator"
  groups  = "system:masters"
}

# On a posture-mode tailnet, such as the one in the p0_tailscale_tailnet
# example's Google Cloud variant, every destination is a P0 posture. There
# tailscale_acl.gitops defines the posture, posture:p0_prod_db with the
# assertion custom:p0_prod_db == true, and gates a grant on it with srcPosture.
# P0 checks that the posture exists, so the destination depends on the
# tailscale_acl that defines it.
resource "p0_tailscale_destination" "prod_db_posture" {
  tailnet = p0_tailscale_tailnet.gcp_example.id
  name    = "prod-db"
  type    = "posture"
  target  = "posture:p0_prod_db"

  depends_on = [tailscale_acl.gitops]
}
