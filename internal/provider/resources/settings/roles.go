// Copyright (c) 2025 P0 Security, Inc
// SPDX-License-Identifier: MPL-2.0

package settings

import "github.com/hashicorp/terraform-plugin-framework/resource"

// Role descriptions, mirroring the P0 app Admin page
// (frontend/src/components/Admin/Admin.tsx). The backend role slug for each is
// noted in the code comments below.
const (
	ownerRoleDescription            = "**Owners** can add integrations and alter organization settings."
	securityReviewerRoleDescription = "**Security Reviewers** can review policies and user access, and may be configured as approvers for access requests."
	assessmentRoleDescription       = "**Assessment Users** can run, manage, and view IAM assessments."
	assessmentViewerRoleDescription = "**Assessment Viewers** can view IAM assessments."
)

// readLimitationNote is appended to every role-binding resource description
// because the P0 settings API exposes no read endpoint for role bindings.
const readLimitationNote = "\n\nThe P0 API does not expose a read endpoint for role bindings, so Terraform cannot detect changes made outside of Terraform (drift). Manage each binding exclusively through Terraform."

const (
	userAttrDescription  = "The user's email address."
	groupAttrDescription = "The directory group identifier."
)

// newUserBinding constructs a role-binding resource for an individual user.
func newUserBinding(typeName, role, roleDescription string) func() resource.Resource {
	return func() resource.Resource {
		return &roleBinding{
			typeName:            typeName,
			role:                role,
			kind:                "users",
			attr:                "email",
			attrDescription:     userAttrDescription,
			markdownDescription: roleDescription + readLimitationNote,
		}
	}
}

// newGroupBinding constructs a role-binding resource for a directory group.
func newGroupBinding(typeName, role, roleDescription string) func() resource.Resource {
	return func() resource.Resource {
		return &roleBinding{
			typeName:            typeName,
			role:                role,
			kind:                "groups",
			attr:                "group",
			attrDescription:     groupAttrDescription,
			markdownDescription: roleDescription + readLimitationNote,
		}
	}
}

// Owners (backend role "owner").
var NewOwnerUser = newUserBinding("owner_user", "owner", ownerRoleDescription)
var NewOwnerGroup = newGroupBinding("owner_group", "owner", ownerRoleDescription)

// Security Reviewers (backend role "manager").
var NewSecurityReviewerUser = newUserBinding("security_reviewer_user", "manager", securityReviewerRoleDescription)
var NewSecurityReviewerGroup = newGroupBinding("security_reviewer_group", "manager", securityReviewerRoleDescription)

// Assessment Users (backend role "iamOwner").
var NewAssessmentUser = newUserBinding("assessment_user", "iamOwner", assessmentRoleDescription)
var NewAssessmentGroup = newGroupBinding("assessment_group", "iamOwner", assessmentRoleDescription)

// Assessment Viewers (backend role "iamViewer").
var NewAssessmentViewerUser = newUserBinding("assessment_viewer_user", "iamViewer", assessmentViewerRoleDescription)
var NewAssessmentViewerGroup = newGroupBinding("assessment_viewer_group", "iamViewer", assessmentViewerRoleDescription)
