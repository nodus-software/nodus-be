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
	TenantID            string     `json:"tenant_id"`
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
	DeactivatedAt       *time.Time `json:"deactivated_at,omitempty"`
	DeactivatedBy       *string    `json:"deactivated_by,omitempty"`
	DeactivationReason  *string    `json:"deactivation_reason,omitempty"`
	SuspendedAt         *time.Time `json:"suspended_at,omitempty"`
	SuspendedBy         *string    `json:"suspended_by,omitempty"`
	SuspensionReason    *string    `json:"suspension_reason,omitempty"`
	InvitationExpiresAt *time.Time `json:"invitation_expires_at,omitempty"`
	InvitationStatus    *string    `json:"invitation_status,omitempty"`
}

func toUserProfileResponse(u User) UserProfileResponse {
	response := UserProfileResponse{
		ID: u.ID, TenantID: u.TenantID, FullName: u.FullName, Username: u.Username, Email: u.Email,
		ProviderIdentifier: u.ProviderIdentifier, Roles: u.RoleNames, Permissions: u.Permissions,
		Status: string(u.Status), MFAEnrolled: u.MFAEnrolled,
		LastAccessReviewAt: u.LastAccessReviewAt, NextAccessReviewDue: u.NextAccessReviewDue,
		DeactivatedAt:       u.DeactivatedAt,
		DeactivatedBy:       u.DeactivatedBy,
		DeactivationReason:  u.DeactivationReason,
		SuspendedAt:         u.SuspendedAt,
		SuspendedBy:         u.SuspendedBy,
		SuspensionReason:    u.SuspensionReason,
		InvitationExpiresAt: u.InvitationExpiresAt,
	}
	if u.Status == StatusInvited && u.InvitationExpiresAt != nil {
		status := "valid"
		if u.InvitationUsedAt != nil || time.Now().After(*u.InvitationExpiresAt) {
			status = "expired"
		}
		response.InvitationStatus = &status
	}
	return response
}

// UpdateUserRequest is the PATCH /users/{userId} body. RoleIDs is only
// applied when non-empty (nil/empty means "leave roles unchanged").
// ProviderIdentifier is not part of the documented schema but is required
// by the contract's own description of what happens when a user's roles
// change from non-clinical to clinical — accepted here as an additional
// optional field for that case. Status remains decode-compatible during the
// API transition, but the service rejects it; lifecycle changes use the
// explicit suspend, restore, deactivate, and reactivation endpoints.
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

type LifecycleReasonRequest struct {
	Reason string `json:"reason" validate:"required,max=500"`
}

type TemporaryRestrictionResponse struct {
	Mechanism     string     `json:"mechanism"`
	FailureCount  int        `json:"failure_count"`
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty"`
	LockedUntil   *time.Time `json:"locked_until,omitempty"`
}

type SecurityStatusResponse struct {
	UserID                string                         `json:"user_id"`
	Status                string                         `json:"status"`
	SuspendedAt           *time.Time                     `json:"suspended_at,omitempty"`
	SuspendedBy           *string                        `json:"suspended_by,omitempty"`
	SuspensionReason      *string                        `json:"suspension_reason,omitempty"`
	DeactivatedAt         *time.Time                     `json:"deactivated_at,omitempty"`
	DeactivatedBy         *string                        `json:"deactivated_by,omitempty"`
	DeactivationReason    *string                        `json:"deactivation_reason,omitempty"`
	TemporaryRestrictions []TemporaryRestrictionResponse `json:"temporary_restrictions"`
}
