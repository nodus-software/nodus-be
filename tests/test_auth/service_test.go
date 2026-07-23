package test_auth

import (
	"testing"

	"nodus-health/internal/auth"
)

// TestValidatePasswordPolicy exercises the pure policy-validation logic
// directly, without a database, per "Repositories should have interfaces...
// business logic should be unit-test friendly."
func TestValidatePasswordPolicy(t *testing.T) {
	policy := auth.PasswordPolicy{
		MinLength: 12, RequireUppercase: true, RequireNumber: true,
		RequireSymbol: true, RejectCommonPasswords: true,
	}

	cases := []struct {
		name       string
		password   string
		wantIssues bool
	}{
		{"valid strong password", "Str0ng!Passw0rd", false},
		{"too short", "Sh0rt!", true},
		{"missing uppercase", "nouppercase1!", true},
		{"missing number", "NoNumberHere!", true},
		{"missing symbol", "NoSymbolHere1", true},
		{"common password", "password", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			violations := auth.ValidatePasswordPolicy(tc.password, policy)
			if tc.wantIssues && len(violations) == 0 {
				t.Fatalf("expected violations for %q, got none", tc.password)
			}
			if !tc.wantIssues && len(violations) != 0 {
				t.Fatalf("expected no violations for %q, got %v", tc.password, violations)
			}
		})
	}
}
