// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package accesspolicy

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func prefixDescription(version int, description string) string {
	if version == 0 {
		return `May only be used if 'type' is 'group'. ` + description
	}
	return description
}

func DirectoryAttribute(version int) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: prefixDescription(version, `One of "azure-ad", "entra-id", "okta", or "workspace".`),
		Required:            true,
	}
}

func IdAttribute(version int) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: prefixDescription(version, `This is the directory's internal group identifier.`),
		Required:            true,
	}
}

func LabelAttribute(version int) schema.StringAttribute {
	// The P0 API requires a label on every group (IdpGroup requires id, label,
	// and directory), so it is required alongside id and directory. Version 0
	// stored it as an optional attribute; keep that shape so old state still
	// decodes during upgrades and `moved` migrations.
	return schema.StringAttribute{
		MarkdownDescription: prefixDescription(version, `This is any human-readable name for the directory group specified in the 'id' attribute.`),
		Required:            version >= 1,
		Optional:            version < 1,
	}
}

func AttachGroupAttributes(version int64, attributes map[string]schema.Attribute) map[string]schema.Attribute {
	switch version {
	case 0:
		{
			attributes["directory"] = DirectoryAttribute(0)
			attributes["id"] = IdAttribute(0)
			attributes["label"] = LabelAttribute(0)
		}
	default:
		{
			attributes["groups"] = schema.ListNestedAttribute{
				MarkdownDescription: `Required, and may only be used, if 'type' is 'group'. If the user is a member of any of these groups, the rule will match.`,
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"directory": DirectoryAttribute(1),
						"id":        IdAttribute(1),
						"label":     LabelAttribute(1),
					},
				},
			}
		}
	}
	return attributes
}

func AttachGroupFilterEffectAttribute(version int64, attributes map[string]schema.Attribute) map[string]schema.Attribute {
	switch version {
	case 0:
	case 1:
		return attributes
	default:
		{
			attributes["effect"] = schema.StringAttribute{
				MarkdownDescription: `Required, and may only be used, if 'type' is 'group'. The filter effect. May be one of:
	 - 'keep': Access rule only applies when a requestor is a member of any of the specified groups
	 - 'remove': Access rule only applies when a requestor is _not_ a member of any of the specified groups`,
				Optional: true,
			}
		}
	}
	return attributes
}
