package invitation

import "errors"

var (
	ErrRoleNotFound               = errors.New("one or more roles not found")
	ErrEmailAlreadyExists         = errors.New("an account or pending invitation already exists for this email")
	ErrProviderIdentifierRequired = errors.New("provider_identifier is required for the selected role(s)")
	ErrTokenInvalid               = errors.New("invitation invalid, expired, or already used")
	ErrTokenExpired               = errors.New("invitation expired")
	ErrPasswordPolicyViolation    = errors.New("password does not meet policy requirements")
	ErrUserNotFound               = errors.New("user not found")
	ErrNotPending                 = errors.New("invitation is not pending")
	ErrNotDeactivated             = errors.New("account is not deactivated")
	ErrReactivationTokenInvalid   = errors.New("reactivation link invalid, expired, or already used")
	ErrReactivationTokenExpired   = errors.New("reactivation link expired")
)

// PolicyViolationError wraps ErrPasswordPolicyViolation with the specific
// rules that failed, for the 400 response's details field.
type PolicyViolationError struct {
	Violations []string
}

func (e *PolicyViolationError) Error() string { return ErrPasswordPolicyViolation.Error() }
func (e *PolicyViolationError) Unwrap() error { return ErrPasswordPolicyViolation }
