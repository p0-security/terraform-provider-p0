// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

// Package duration defines P0's shared duration representation: the
// `{time, unit}` object, its schema attributes, and the backend's derived
// duration label. It is consumed by the settings and access_policy resources.
package duration

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// Option is the wire + state representation of a P0 duration. `Time` is an
// integer count of `Unit`s. The P0 API also stores a derived `value` label, but
// as it is fully determined by (time, unit) we compute it on demand (see
// ComputeValue) rather than tracking it in Terraform state.
type Option struct {
	Time int64  `json:"time" tfsdk:"time"`
	Unit string `json:"unit" tfsdk:"unit"`
}

// unitLabels mirrors `timeUnitLabels` in the P0 app
// (shared/src/permission-requests/util.ts).
var unitLabels = map[string]string{
	"s": "second",
	"m": "minute",
	"h": "hour",
	"d": "day",
	"w": "week",
}

// units is the set of accepted `unit` values, in the order P0 declares them.
var units = []string{"s", "m", "h", "d", "w"}

// ComputeValue reproduces the P0 backend's derived duration label
// (convertDurationToDurationOption in shared/src/permission-requests/util.ts):
// "<time> <unit-label>", pluralized when time != 1. This is the key P0 uses to
// identify an expiry option for deletion.
func ComputeValue(time int64, unit string) string {
	label := unitLabels[unit]
	if time != 1 {
		label += "s"
	}
	return fmt.Sprintf("%d %s", time, label)
}

// Attributes are the attributes shared by every duration object.
func Attributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"time": schema.Int64Attribute{
			MarkdownDescription: "The number of `unit`s in this duration. Must be a positive integer.",
			Required:            true,
			Validators: []validator.Int64{
				int64validator.AtLeast(1),
			},
		},
		"unit": schema.StringAttribute{
			MarkdownDescription: "The duration unit. One of `s` (seconds), `m` (minutes), `h` (hours), `d` (days), or `w` (weeks).",
			Required:            true,
			Validators: []validator.String{
				stringvalidator.OneOf(units...),
			},
		},
	}
}

// RequiredAttribute is a single required duration object (e.g. the approvable
// duration).
func RequiredAttribute(markdownDescription string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: markdownDescription,
		Required:            true,
		Attributes:          Attributes(),
	}
}
