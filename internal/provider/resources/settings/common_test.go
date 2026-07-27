// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package settings

import "testing"

// TestComputeValue verifies that the derived duration label matches the P0
// backend's convertDurationToDurationOption output, which is the key P0 uses to
// identify an expiry option for deletion.
func TestComputeValue(t *testing.T) {
	cases := []struct {
		time int64
		unit string
		want string
	}{
		{1, "s", "1 second"},
		{5, "m", "5 minutes"},
		{1, "h", "1 hour"},
		{24, "h", "24 hours"},
		{168, "h", "168 hours"},
		{1, "d", "1 day"},
		{180, "d", "180 days"},
		{1, "w", "1 week"},
		{2, "w", "2 weeks"},
	}
	for _, c := range cases {
		got := computeValue(c.time, c.unit)
		if got != c.want {
			t.Errorf("computeValue(%d, %q) = %q; want %q", c.time, c.unit, got, c.want)
		}
	}
}
