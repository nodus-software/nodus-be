package auth

import "time"

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type LoginChallengeResponse struct {
	ChallengeToken string   `json:"challenge_token"`
	MFAMethods     []string `json:"mfa_methods"`
}

type AccountLockedResponse struct {
	LockedUntil time.Time `json:"locked_until"`
	Reason      string    `json:"reason"`
}

type VerifyMFARequest struct {
	ChallengeToken string `json:"challenge_token" validate:"required"`
	Method         string `json:"method" validate:"required,oneof=totp biometric"`
	Code           string `json:"code" validate:"required"`
}

type TokenPairResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
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
}

type TOTPSetupResponse struct {
	Secret      string   `json:"secret"`
	QRCodeURI   string   `json:"qr_code_uri"`
	BackupCodes []string `json:"backup_codes"`
}

type ConfirmTOTPRequest struct {
	Code string `json:"code" validate:"required"`
}

type RegisterBiometricRequest struct {
	DevicePublicKey string `json:"device_public_key" validate:"required"`
	DeviceLabel     string `json:"device_label"`
}

type MFAFactorResponse struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required"`
}

type PasswordPolicyResponse struct {
	MinLength             int  `json:"min_length"`
	RequireUppercase      bool `json:"require_uppercase"`
	RequireNumber         bool `json:"require_number"`
	RequireSymbol         bool `json:"require_symbol"`
	RejectCommonPasswords bool `json:"reject_common_passwords"`
	MaxAgeDays            int  `json:"max_age_days"`
}

type RequestPasswordResetRequest struct {
	Username string `json:"username" validate:"required"`
}

type ConfirmPasswordResetRequest struct {
	ResetToken  string `json:"reset_token" validate:"required"`
	NewPassword string `json:"new_password" validate:"required"`
}

type SessionResponse struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	DeviceLabel  string    `json:"device_label"`
	IPAddress    string    `json:"ip_address"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	Current      bool      `json:"current"`
}
