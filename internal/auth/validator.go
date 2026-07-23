package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"

	"nodus-health/pkg/response"
)

var validate = validator.New()

// bindJSON decodes the request body into T and runs struct-tag validation.
// On failure it writes the appropriate error response itself and returns
// ok=false so the caller can return immediately.
func bindJSON[T any](w http.ResponseWriter, r *http.Request) (payload T, ok bool) {
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		response.BadRequest(w, "invalid request body")
		return payload, false
	}
	if err := validate.Struct(payload); err != nil {
		verrs, ok := err.(validator.ValidationErrors)
		if !ok {
			response.BadRequest(w, "invalid request body")
			return payload, false
		}
		response.Validation(w, formatValidationErrors(verrs))
		return payload, false
	}
	return payload, true
}

func formatValidationErrors(verrs validator.ValidationErrors) map[string]string {
	details := make(map[string]string, len(verrs))
	for _, fe := range verrs {
		field := strings.ToLower(fe.Field())
		switch fe.Tag() {
		case "required":
			details[field] = fmt.Sprintf("%s is required", field)
		case "oneof":
			details[field] = fmt.Sprintf("%s must be one of: %s", field, fe.Param())
		case "email":
			details[field] = fmt.Sprintf("%s must be a valid email address", field)
		default:
			details[field] = fmt.Sprintf("%s is invalid", field)
		}
	}
	return details
}

// ValidatePasswordPolicy checks password against the configured complexity
// policy and returns the human-readable list of violated rules (empty if
// the password satisfies every rule).
func ValidatePasswordPolicy(password string, policy PasswordPolicy) []string {
	var violations []string

	if len(password) < policy.MinLength {
		violations = append(violations, fmt.Sprintf("must be at least %d characters", policy.MinLength))
	}
	if policy.RequireUppercase && !containsFunc(password, unicode.IsUpper) {
		violations = append(violations, "must contain an uppercase letter")
	}
	if policy.RequireNumber && !containsFunc(password, unicode.IsDigit) {
		violations = append(violations, "must contain a number")
	}
	if policy.RequireSymbol && !containsFunc(password, isSymbol) {
		violations = append(violations, "must contain a symbol")
	}
	if policy.RejectCommonPasswords && isCommonPassword(password) {
		violations = append(violations, "must not be a commonly used password")
	}

	return violations
}

func containsFunc(s string, f func(rune) bool) bool {
	for _, r := range s {
		if f(r) {
			return true
		}
	}
	return false
}

func isSymbol(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

var commonPasswords = map[string]struct{}{
	"password": {}, "password1": {}, "123456": {}, "12345678": {}, "123456789": {},
	"qwerty": {}, "qwerty123": {}, "letmein": {}, "welcome": {}, "admin": {},
	"admin123": {}, "changeme": {}, "iloveyou": {}, "111111": {}, "abc123": {},
}

func isCommonPassword(password string) bool {
	_, ok := commonPasswords[strings.ToLower(password)]
	return ok
}
