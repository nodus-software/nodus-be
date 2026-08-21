package auth

import (
	"errors"
	"time"
)

var (
	ErrInvalidCredentials        = errors.New("invalid email or password")
	ErrAccountLocked             = errors.New("account is locked")
	ErrMFANotEnrolled            = errors.New("no confirmed mfa factor enrolled")
	ErrChallengeInvalid          = errors.New("challenge token invalid")
	ErrChallengeExpired          = errors.New("challenge token expired")
	ErrMFACodeInvalid            = errors.New("mfa code invalid")
	ErrRefreshTokenInvalid       = errors.New("refresh token invalid")
	ErrRefreshTokenRevoked       = errors.New("refresh token revoked")
	ErrFactorNotFound            = errors.New("mfa factor not found")
	ErrLastFactorRemaining       = errors.New("cannot remove the last remaining mfa factor")
	ErrInvalidPublicKey          = errors.New("invalid device public key")
	ErrPasswordPolicyViolation   = errors.New("password does not meet policy requirements")
	ErrCurrentPasswordInvalid    = errors.New("current password is incorrect")
	ErrResetTokenInvalid         = errors.New("reset token invalid, expired, or already used")
	ErrRecoveryTokenInvalid      = errors.New("recovery token invalid, expired, or already used")
	ErrRateLimitExceeded         = errors.New("rate limit exceeded")
	ErrSessionNotFound           = errors.New("session not found")
	ErrUserNotFound              = errors.New("user not found")
	ErrPermissionDenied          = errors.New("permission denied")
	ErrEnrollmentTokenInvalid    = errors.New("enrollment token invalid, expired, or already used")
	ErrWebAuthnInvalid           = errors.New("passkey verification failed")
	ErrWebAuthnUnavailable       = errors.New("passkey authentication is unavailable")
	ErrTOTPAlreadyEnrolled       = errors.New("an authenticator app is already enrolled")
	ErrAuthenticationChallenge   = errors.New("additional verification required")
	ErrAuthenticationDelayed     = errors.New("authentication temporarily delayed")
	ErrAuthenticationUnavailable = errors.New("authentication temporarily unavailable")
)

type RetryError struct {
	Cause      error
	RetryAfter time.Duration
}

func (e *RetryError) Error() string { return e.Cause.Error() }
func (e *RetryError) Unwrap() error { return e.Cause }

// LockedError wraps ErrAccountLocked with the lockout expiry so the handler
// can surface locked_until in the 423 response body.
type LockedError struct {
	LockedUntil time.Time
}

func (e *LockedError) Error() string { return ErrAccountLocked.Error() }
func (e *LockedError) Unwrap() error { return ErrAccountLocked }

// PolicyViolationError wraps ErrPasswordPolicyViolation with the specific
// rules that failed, for the 422 response's details field.
type PolicyViolationError struct {
	Violations []string
}

func (e *PolicyViolationError) Error() string { return ErrPasswordPolicyViolation.Error() }
func (e *PolicyViolationError) Unwrap() error { return ErrPasswordPolicyViolation }
