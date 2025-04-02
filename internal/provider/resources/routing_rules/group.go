// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package routingrules

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
		MarkdownDescription: prefixDescription(version, `One of "azure-ad", "okta", or "workspace".`),
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
	return schema.StringAttribute{
		MarkdownDescription: prefixDescription(version, `This is any human-readable name for the directory group specified in the 'id' attribute.`),
		Optional:            true,
	}
}

// var GroupAttribute = schema.NestedAttributeObject{
// 	Attributes: map[string]schema.Attribute{
// 		"directory": DirectoryAttribute(1),
// 		"id":        IdAttribute(1),
// 		"label":     LabelAttribute(1),
// 	},
// }

// var GroupsAttribute = schema.ListNestedAttribute{
// 	MarkdownDescription: `May only be used if 'type' is 'group'. This is the list of groups that the requestor must be a member of to match.`,
// 	Optional:            true,
// 	NestedObject:        GroupAttribute,
// }

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
				MarkdownDescription: `May only be used if 'type' is 'group'. If the user is a member of any of these groups, the rule will match.`,
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
