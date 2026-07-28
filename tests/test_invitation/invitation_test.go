package test_invitation

import (
	"net/http"
	"strings"
	"testing"

	"nodus-health/internal/invitation"
	"nodus-health/pkg/utility"
)

func TestInvite_GoldenPath_CreatesPendingUserAndSendsEmail(t *testing.T) {
	env := Setup(t)
	env.CreateRole("role-receptionist", "Receptionist", false)
	_, actorToken := env.NewActor(t, "users:invite")

	rec := env.JSON(t, http.MethodPost, "/users/invitations", actorToken, invitation.InviteUserRequest{
		FullName: "Jane Doe", Email: "jane@example.com", RoleIDs: []string{"role-receptionist"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var profile invitation.UserProfileResponse
	Decode(t, rec, &profile)
	if profile.Status != "invited" {
		t.Fatalf("expected status=invited, got %s", profile.Status)
	}
	if env.Mailer.LastToken() == "" {
		t.Fatal("expected an invitation email to be sent with a token")
	}
	mail := env.Mailer.Sent[len(env.Mailer.Sent)-1]
	if !strings.Contains(mail.Text, "https://app.test/invite?token=") || !strings.Contains(mail.Text, "&tenant=nodus-test") {
		t.Fatalf("expected password-setup URL in text email, got %q", mail.Text)
	}
	if !strings.Contains(mail.HTML, "Accept invitation") || !strings.Contains(mail.HTML, "https://app.test/invite?token=") || !strings.Contains(mail.HTML, "&amp;tenant=nodus-test") {
		t.Fatal("expected a rendered HTML invitation with a password-setup link")
	}
}

func TestReactivation_GoldenPathRequiresFreshSetupAndIsSingleUse(t *testing.T) {
	env := Setup(t)
	userID, err := utility.GenerateUUID()
	if err != nil {
		t.Fatal(err)
	}
	env.Repo.users[userID] = invitation.PendingUser{
		ID: userID, FullName: "Returning User", Email: "returning@example.com",
		Status: invitation.UserStatusDeactivated, PasswordSet: true,
	}
	env.Repo.usersByMail["returning@example.com"] = userID
	_, actorToken := env.NewActor(t, "users:deactivate")

	request := env.JSON(t, http.MethodPost, "/users/"+userID+"/reactivate", actorToken, invitation.LifecycleReasonRequest{
		Reason: "rehired",
	})
	if request.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", request.Code, request.Body.String())
	}
	rawToken := env.Mailer.LastToken()
	if rawToken == "" {
		t.Fatal("expected a reactivation token in email")
	}
	if env.Repo.users[userID].Status != invitation.UserStatusDeactivated {
		t.Fatal("expected account to remain deactivated until link acceptance")
	}

	preview := env.JSON(t, http.MethodGet, "/users/reactivations/"+rawToken, "", nil)
	if preview.Code != http.StatusOK {
		t.Fatalf("expected 200 previewing reactivation, got %d: %s", preview.Code, preview.Body.String())
	}
	accept := env.JSON(t, http.MethodPost, "/users/reactivations/"+rawToken+"/accept", "", invitation.AcceptReactivationRequest{
		Token: rawToken, Password: "Str0ng!Passw0rd",
	})
	if accept.Code != http.StatusOK {
		t.Fatalf("expected 200 accepting reactivation, got %d: %s", accept.Code, accept.Body.String())
	}
	if env.Repo.users[userID].Status != invitation.UserStatusActive || len(env.Repo.enrollments) != 1 {
		t.Fatal("expected active account and a fresh MFA enrollment token")
	}
	replay := env.JSON(t, http.MethodPost, "/users/reactivations/"+rawToken+"/accept", "", invitation.AcceptReactivationRequest{
		Token: rawToken, Password: "Str0ng!Passw0rd",
	})
	if replay.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 replaying reactivation, got %d: %s", replay.Code, replay.Body.String())
	}
}

func TestCancelInvitation_RemovesOnlyPendingAccount(t *testing.T) {
	env := Setup(t)
	env.CreateRole("role-receptionist", "Receptionist", false)
	_, actorToken := env.NewActor(t, "users:invite")
	inviteRec := env.JSON(t, http.MethodPost, "/users/invitations", actorToken, invitation.InviteUserRequest{
		FullName: "Cancelled User", Email: "cancelled@example.com", RoleIDs: []string{"role-receptionist"},
	})
	var profile invitation.UserProfileResponse
	Decode(t, inviteRec, &profile)

	cancel := env.JSON(t, http.MethodPost, "/users/invitations/"+profile.ID+"/cancel", actorToken, invitation.LifecycleReasonRequest{
		Reason: "invited in error",
	})
	if cancel.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", cancel.Code, cancel.Body.String())
	}
	if _, ok := env.Repo.users[profile.ID]; ok {
		t.Fatal("expected never-activated pending account to be removed")
	}
}

func TestInvite_ClinicalRoleWithoutProviderIdentifier_Returns422(t *testing.T) {
	env := Setup(t)
	env.CreateRole("role-nurse", "Nurse", true)
	_, actorToken := env.NewActor(t, "users:invite")

	rec := env.JSON(t, http.MethodPost, "/users/invitations", actorToken, invitation.InviteUserRequest{
		FullName: "Nurse Joy", Email: "joy@example.com", RoleIDs: []string{"role-nurse"},
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInvite_DuplicatePendingEmail_ResendsInsteadOfCreatingAnotherUser(t *testing.T) {
	env := Setup(t)
	env.CreateRole("role-receptionist", "Receptionist", false)
	_, actorToken := env.NewActor(t, "users:invite")

	req := invitation.InviteUserRequest{FullName: "Jane Doe", Email: "jane@example.com", RoleIDs: []string{"role-receptionist"}}
	first := env.JSON(t, http.MethodPost, "/users/invitations", actorToken, req)
	if first.Code != http.StatusCreated {
		t.Fatalf("expected first invite to succeed, got %d: %s", first.Code, first.Body.String())
	}
	firstToken := env.Mailer.LastToken()
	userCount := len(env.Repo.users)

	second := env.JSON(t, http.MethodPost, "/users/invitations", actorToken, req)
	if second.Code != http.StatusCreated {
		t.Fatalf("expected a replacement invitation, got %d: %s", second.Code, second.Body.String())
	}
	if len(env.Repo.users) != userCount {
		t.Fatal("expected the existing pending user to be reused")
	}
	if env.Mailer.LastToken() == firstToken {
		t.Fatal("expected a new invitation token")
	}
}

func TestValidateToken_UnknownToken_Returns400(t *testing.T) {
	env := Setup(t)
	rec := env.JSON(t, http.MethodGet, "/users/invitations/does-not-exist", "", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAcceptInvite_GoldenPath_ActivatesUserAndIssuesEnrollmentToken(t *testing.T) {
	env := Setup(t)
	env.CreateRole("role-receptionist", "Receptionist", false)
	_, actorToken := env.NewActor(t, "users:invite")

	env.JSON(t, http.MethodPost, "/users/invitations", actorToken, invitation.InviteUserRequest{
		FullName: "Jane Doe", Email: "jane@example.com", RoleIDs: []string{"role-receptionist"},
	})
	rawToken := env.Mailer.LastToken()
	if rawToken == "" {
		t.Fatal("expected an invitation token to have been emailed")
	}

	preview := env.JSON(t, http.MethodGet, "/users/invitations/"+rawToken, "", nil)
	if preview.Code != http.StatusOK {
		t.Fatalf("expected 200 previewing invite, got %d: %s", preview.Code, preview.Body.String())
	}

	acceptRec := env.JSON(t, http.MethodPost, "/users/invitations/"+rawToken+"/accept", "", invitation.AcceptInviteRequest{
		Token: rawToken, Password: "Str0ng!Passw0rd",
	})
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", acceptRec.Code, acceptRec.Body.String())
	}
	var resp invitation.EnrollmentTokenResponse
	Decode(t, acceptRec, &resp)
	if resp.EnrollmentToken == "" {
		t.Fatal("expected a non-empty enrollment token")
	}

	// The invite token is single-use: accepting again must fail.
	replay := env.JSON(t, http.MethodPost, "/users/invitations/"+rawToken+"/accept", "", invitation.AcceptInviteRequest{
		Token: rawToken, Password: "Str0ng!Passw0rd",
	})
	if replay.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 replaying a consumed invite token, got %d: %s", replay.Code, replay.Body.String())
	}
}

func TestAcceptInvite_WeakPassword_Returns400(t *testing.T) {
	env := Setup(t)
	env.CreateRole("role-receptionist", "Receptionist", false)
	_, actorToken := env.NewActor(t, "users:invite")

	env.JSON(t, http.MethodPost, "/users/invitations", actorToken, invitation.InviteUserRequest{
		FullName: "Jane Doe", Email: "jane@example.com", RoleIDs: []string{"role-receptionist"},
	})
	rawToken := env.Mailer.LastToken()

	rec := env.JSON(t, http.MethodPost, "/users/invitations/"+rawToken+"/accept", "", invitation.AcceptInviteRequest{
		Token: rawToken, Password: "weak",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a weak password, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestResendInvite_IssuesNewTokenAndInvalidatesOldOne(t *testing.T) {
	env := Setup(t)
	env.CreateRole("role-receptionist", "Receptionist", false)
	_, actorToken := env.NewActor(t, "users:invite")

	createRec := env.JSON(t, http.MethodPost, "/users/invitations", actorToken, invitation.InviteUserRequest{
		FullName: "Jane Doe", Email: "jane@example.com", RoleIDs: []string{"role-receptionist"},
	})
	var profile invitation.UserProfileResponse
	Decode(t, createRec, &profile)
	oldToken := env.Mailer.LastToken()

	resendRec := env.JSON(t, http.MethodPost, "/users/invitations/"+profile.ID+"/resend", actorToken, nil)
	if resendRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resendRec.Code, resendRec.Body.String())
	}
	newToken := env.Mailer.LastToken()
	if newToken == "" || newToken == oldToken {
		t.Fatalf("expected a new, different invitation token; old=%q new=%q", oldToken, newToken)
	}

	// The old token must no longer be usable.
	oldAccept := env.JSON(t, http.MethodPost, "/users/invitations/"+oldToken+"/accept", "", invitation.AcceptInviteRequest{
		Token: oldToken, Password: "Str0ng!Passw0rd",
	})
	if oldAccept.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 accepting an invalidated token, got %d: %s", oldAccept.Code, oldAccept.Body.String())
	}

	// The new token must work.
	newAccept := env.JSON(t, http.MethodPost, "/users/invitations/"+newToken+"/accept", "", invitation.AcceptInviteRequest{
		Token: newToken, Password: "Str0ng!Passw0rd",
	})
	if newAccept.Code != http.StatusOK {
		t.Fatalf("expected 200 accepting the resent token, got %d: %s", newAccept.Code, newAccept.Body.String())
	}
}
