package users

import "time"

// ListUsersFilter mirrors the query parameters on GET /users. A nil field
// means "no filter"; StaleAccess=true means next_access_review_due is
// missing or in the past.
type ListUsersFilter struct {
	Role        *string
	Status      *string
	StaleAccess *bool
}

type UserProfileResponse struct {
	ID                  string     `json:"id"`
	FullName            string     `json:"full_name"`
	Username            string     `json:"username"`
	Email               string     `json:"email"`
	ProviderIdentifier  *string    `json:"provider_identifier,omitempty"`
	Roles               []string   `json:"roles"`
	Permissions         []string   `json:"permissions"`
	Status              string     `json:"status"`
	MFAEnrolled         bool       `json:"mfa_enrolled"`
	LastAccessReviewAt  *time.Time `json:"last_access_review_at,omitempty"`
	NextAccessReviewDue *time.Time `json:"next_access_review_due,omitempty"`
}

func toUserProfileResponse(u User) UserProfileResponse {
	return UserProfileResponse{
		ID: u.ID, FullName: u.FullName, Username: u.Username, Email: u.Email,
		ProviderIdentifier: u.ProviderIdentifier, Roles: u.RoleNames, Permissions: u.Permissions,
		Status: string(u.Status), MFAEnrolled: u.MFAEnrolled,
		LastAccessReviewAt: u.LastAccessReviewAt, NextAccessReviewDue: u.NextAccessReviewDue,
	}
}

// UpdateUserRequest is the PATCH /users/{userId} body. RoleIDs is only
// applied when non-empty (nil/empty means "leave roles unchanged").
// ProviderIdentifier is not part of the documented schema but is required
// by the contract's own description of what happens when a user's roles
// change from non-clinical to clinical — accepted here as an additional
// optional field for that case.
type UpdateUserRequest struct {
	RoleIDs            []string `json:"role_ids"`
	Status             *string  `json:"status" validate:"omitempty,oneof=active suspended"`
	ProviderIdentifier *string  `json:"provider_identifier"`
}

type AccessReviewRequest struct {
	Decision   string `json:"decision" validate:"required,oneof=confirm_access revoke_access modify_access"`
	ReviewedBy string `json:"reviewed_by" validate:"required"`
	Notes      string `json:"notes"`
}

type UnlockRequest struct {
	Reason string `json:"reason"`
}
