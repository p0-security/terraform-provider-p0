# A bundle's policy rule was imported as p0_access_policy before
# p0_access_bundle existed. Remove the rule from state without deleting it...
removed {
  from = p0_access_policy.deploy_bundle

  lifecycle {
    destroy = false
  }
}

# ...and import the bundle that owns it, by the ID after "bundle:".
import {
  to = p0_access_bundle.deploy
  id = "dsy9eYJIUoyBM1RJuvUx"
}
