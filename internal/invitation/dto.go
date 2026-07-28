package invitation

type InviteUserRequest struct {
	FullName           string   `json:"full_name" validate:"required"`
	Email              string   `json:"email" validate:"required,email"`
	ProviderIdentifier string   `json:"provider_identifier"`
	RoleIDs            []string `json:"role_ids" validate:"required,min=1,dive,required"`
}

// UserProfileResponse mirrors the contract's UserProfile shape for the
// invited user this endpoint creates.
type UserProfileResponse struct {
	ID                 string   `json:"id"`
	TenantID           string   `json:"tenant_id"`
	FullName           string   `json:"full_name"`
	Username           string   `json:"username"`
	Email              string   `json:"email"`
	ProviderIdentifier *string  `json:"provider_identifier,omitempty"`
	Roles              []string `json:"roles"`
	Permissions        []string `json:"permissions"`
	Status             string   `json:"status"`
	MFAEnrolled        bool     `json:"mfa_enrolled"`
}

type ValidateInviteResponse struct {
	FullName     string `json:"full_name"`
	Email        string `json:"email"`
	Organization string `json:"organization"`
}

type AcceptInviteRequest struct {
	Token    string `json:"token" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type EnrollmentTokenResponse struct {
	EnrollmentToken string `json:"enrollment_token"`
}

type LifecycleReasonRequest struct {
	Reason string `json:"reason" validate:"required,max=500"`
}

type ValidateReactivationResponse struct {
	FullName     string `json:"full_name"`
	Email        string `json:"email"`
	Organization string `json:"organization"`
}

type AcceptReactivationRequest struct {
	Token    string `json:"token" validate:"required"`
	Password string `json:"password" validate:"required"`
}
