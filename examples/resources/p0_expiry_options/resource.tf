# Configures the selectable request-duration presets ("expiry options") offered
# to requestors. This is a singleton resource; declare it at most once.
resource "p0_expiry_options" "example" {
  options = [
    { time = 5, unit = "m" },
    { time = 1, unit = "h" },
    { time = 24, unit = "h" },
    { time = 168, unit = "h" },
    { time = 720, unit = "h" },
  ]
}
