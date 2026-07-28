package email

import (
	"strings"
	"testing"
	"time"
)

func TestRendererRendersActiveTemplates(t *testing.T) {
	r := NewRenderer(CommonData{
		AppName: "Nodus Health", AppURL: "https://app.example.test",
		LogoURL:      "https://cdn.example.test/nodus-logo.png",
		SupportEmail: "support@example.test", PrivacyURL: "https://example.test/privacy",
		CurrentYear: 2026,
	})
	expires := time.Date(2026, time.July, 28, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name        string
		render      func() (Rendered, error)
		wantSubject string
		wantURL     string
	}{
		{"organization activation", func() (Rendered, error) {
			return r.RenderOrganizationActivation(OrganizationActivationData{CommonData: CommonData{RecipientName: "Amina", OrganizationName: "Nodus Clinic"}, ActivationURL: "https://app.example.test/activate?t=secret", ExpiresAt: expires})
		}, "Activate your Nodus Health organization", "/activate?t=secret"},
		{"invitation", func() (Rendered, error) {
			return r.RenderStaffInvitation(StaffInvitationData{CommonData: CommonData{RecipientName: "Kamau", OrganizationName: "Nodus Clinic"}, InviterName: "Amina", InviteURL: "https://app.example.test/invite?t=secret", ExpiresAt: expires})
		}, "You've been invited to join Nodus Clinic", "/invite?t=secret"},
		{"resent invitation", func() (Rendered, error) {
			return r.RenderStaffInvitation(StaffInvitationData{CommonData: CommonData{RecipientName: "Kamau", OrganizationName: "Nodus Clinic"}, InviteURL: "https://app.example.test/invite?t=new", ExpiresAt: expires, IsResend: true})
		}, "Your new invitation to Nodus Clinic", "/invite?t=new"},
		{"password reset", func() (Rendered, error) {
			return r.RenderPasswordReset(PasswordResetData{CommonData: CommonData{RecipientName: "Amina"}, ResetURL: "https://app.example.test/reset?t=secret", ExpiresAt: expires})
		}, "Reset your Nodus Health password", "/reset?t=secret"},
		{"account reactivation", func() (Rendered, error) {
			return r.RenderAccountReactivation(AccountReactivationData{CommonData: CommonData{RecipientName: "Amina", OrganizationName: "Nodus Clinic"}, ReactivationURL: "https://app.example.test/reactivate?t=secret", ExpiresAt: expires})
		}, "Reactivate your Nodus Health account", "/reactivate?t=secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.render()
			if err != nil {
				t.Fatal(err)
			}
			if got.Subject != tt.wantSubject {
				t.Fatalf("subject = %q, want %q", got.Subject, tt.wantSubject)
			}
			for format, body := range map[string]string{"HTML": got.HTML, "text": got.Text} {
				if !strings.Contains(body, tt.wantURL) {
					t.Errorf("%s body does not contain action URL", format)
				}
				if strings.Contains(body, "{{") {
					t.Errorf("%s body contains an unresolved template expression", format)
				}
			}
			if !strings.Contains(got.HTML, "#2720ff") {
				t.Error("HTML does not contain the frontend primary brand color")
			}
			if !strings.Contains(got.HTML, "nodus-logo.png") {
				t.Error("HTML does not contain the configured logo")
			}
		})
	}
}
