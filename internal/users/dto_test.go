package users

import (
	"testing"
	"time"
)

func TestUserProfileInvitationStatus(t *testing.T) {
	tests := []struct {
		name       string
		expiresAt  time.Time
		wantStatus string
	}{
		{name: "valid", expiresAt: time.Now().Add(time.Hour), wantStatus: "valid"},
		{name: "expired", expiresAt: time.Now().Add(-time.Hour), wantStatus: "expired"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := toUserProfileResponse(User{
				Status: StatusInvited, InvitationExpiresAt: &tt.expiresAt,
				RoleNames: []string{}, Permissions: []string{},
			})
			if response.InvitationStatus == nil || *response.InvitationStatus != tt.wantStatus {
				t.Fatalf("expected %s invitation, got %v", tt.wantStatus, response.InvitationStatus)
			}
		})
	}
}
