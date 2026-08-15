package auth

import "testing"

func TestTenantWebAuthnRPID(t *testing.T) {
	if got := tenantWebAuthnRPID("localhost", "localhost", "green-clinic"); got != "green-clinic.localhost" {
		t.Fatalf("local tenant RP ID = %q", got)
	}
	if got := tenantWebAuthnRPID("example.com", "example.com", "green-clinic"); got != "example.com" {
		t.Fatalf("production RP ID = %q", got)
	}
}
