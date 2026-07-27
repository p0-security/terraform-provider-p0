# Configures the organization's access-duration policy. This is a singleton
# resource; declare it at most once.
resource "p0_access_durations" "example" {
  # Maximum time between when a request is made and when it can be approved.
  approvable = {
    time = 14
    unit = "d"
  }

  # Maximum duration for which access may be granted.
  max_access = {
    time = 180
    unit = "d"
  }

  # Maximum duration of standing (persistent) access before re-approval.
  standing_access = {
    time = 24
    unit = "h"
  }
}
