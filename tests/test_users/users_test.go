package test_users

import (
	"net/http"
	"testing"

	"nodus-health/internal/users"
)

func TestListUsers_FiltersByStatus(t *testing.T) {
	env := Setup(t)
	env.CreateUser(t, users.StatusActive)
	env.CreateUser(t, users.StatusSuspended)
	_, actorToken := env.NewActor(t, false, "users:read")

	rec := env.JSON(t, http.MethodGet, "/users?status=suspended", actorToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var list []users.UserProfileResponse
	Decode(t, rec, &list)
	if len(list) != 1 || list[0].Status != "suspended" {
		t.Fatalf("expected one suspended user, got %+v", list)
	}
}

func TestUpdateUser_ClinicalRoleWithoutProviderIdentifier_Returns422(t *testing.T) {
	env := Setup(t)
	target := env.CreateUser(t, users.StatusActive)
	env.CreateRole("role-nurse", "Nurse", false, true)
	_, actorToken := env.NewActor(t, false, "users:write")

	rec := env.JSON(t, http.MethodPatch, "/users/"+target, actorToken, users.UpdateUserRequest{
		RoleIDs: []string{"role-nurse"},
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateUser_ClinicalRoleWithProviderIdentifier_Succeeds(t *testing.T) {
	env := Setup(t)
	target := env.CreateUser(t, users.StatusActive)
	env.CreateRole("role-nurse", "Nurse", false, true)
	_, actorToken := env.NewActor(t, false, "users:write")

	providerID := "PRV-001"
	rec := env.JSON(t, http.MethodPatch, "/users/"+target, actorToken, users.UpdateUserRequest{
		RoleIDs: []string{"role-nurse"}, ProviderIdentifier: &providerID,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var profile users.UserProfileResponse
	Decode(t, rec, &profile)
	if len(profile.Roles) != 1 || profile.Roles[0] != "Nurse" {
		t.Fatalf("expected roles=[Nurse], got %v", profile.Roles)
	}
}

func TestUpdateUser_AssigningSuperuserRoleRequiresActorBeSuperuser(t *testing.T) {
	env := Setup(t)
	target := env.CreateUser(t, users.StatusActive)
	env.CreateRole("role-admin", "Superuser", true, false)
	_, nonSuperuserToken := env.NewActor(t, false, "users:write")

	rec := env.JSON(t, http.MethodPatch, "/users/"+target, nonSuperuserToken, users.UpdateUserRequest{
		RoleIDs: []string{"role-admin"},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}

	_, superuserToken := env.NewActor(t, true, "users:write")
	rec2 := env.JSON(t, http.MethodPatch, "/users/"+target, superuserToken, users.UpdateUserRequest{
		RoleIDs: []string{"role-admin"},
	})
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 for superuser actor, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestUnlockUser_NotLocked_Returns409(t *testing.T) {
	env := Setup(t)
	target := env.CreateUser(t, users.StatusActive)
	_, actorToken := env.NewActor(t, false, "users:unlock")

	rec := env.JSON(t, http.MethodPost, "/users/"+target+"/unlock", actorToken, users.UnlockRequest{Reason: "test"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUnlockUser_Locked_Succeeds(t *testing.T) {
	env := Setup(t)
	target := env.CreateUser(t, users.StatusActive)
	env.LockUser(target)
	_, actorToken := env.NewActor(t, false, "users:unlock")

	rec := env.JSON(t, http.MethodPost, "/users/"+target+"/unlock", actorToken, users.UnlockRequest{Reason: "verified with staff member"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRecordAccessReview_RevokeAccessSuspendsUser(t *testing.T) {
	env := Setup(t)
	target := env.CreateUser(t, users.StatusActive)
	_, actorToken := env.NewActor(t, false, "users:review")

	rec := env.JSON(t, http.MethodPost, "/users/"+target+"/access-review", actorToken, users.AccessReviewRequest{
		Decision: "revoke_access", ReviewedBy: "admin-1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var profile users.UserProfileResponse
	Decode(t, rec, &profile)
	if profile.Status != "suspended" {
		t.Fatalf("expected status=suspended after revoke_access, got %s", profile.Status)
	}
}

func TestUsersRoutesRequirePermission(t *testing.T) {
	env := Setup(t)
	_, actorToken := env.NewActor(t, false) // no permissions granted

	rec := env.JSON(t, http.MethodGet, "/users", actorToken, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without users:read, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeactivateUser_RetainsProfileAndChangesStatus(t *testing.T) {
	env := Setup(t)
	target := env.CreateUser(t, users.StatusActive)
	_, actorToken := env.NewActor(t, false, "users:deactivate")

	rec := env.JSON(t, http.MethodPost, "/users/"+target+"/deactivate", actorToken, users.LifecycleReasonRequest{
		Reason: "employment ended",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var profile users.UserProfileResponse
	Decode(t, rec, &profile)
	if profile.Status != "deactivated" || profile.DeactivatedAt == nil || profile.DeactivatedBy == nil || profile.DeactivationReason == nil || *profile.DeactivationReason != "employment ended" {
		t.Fatalf("expected retained deactivated profile, got %+v", profile)
	}
	if _, ok := env.Repo.users[target]; !ok {
		t.Fatal("expected the staff identity row to be retained")
	}
}

func TestDeactivateUser_LastActiveSuperuser_Returns409(t *testing.T) {
	env := Setup(t)
	target := env.CreateUser(t, users.StatusActive)
	env.Repo.superusers[target] = true
	_, actorToken := env.NewActor(t, true, "users:deactivate")

	rec := env.JSON(t, http.MethodPost, "/users/"+target+"/deactivate", actorToken, users.LifecycleReasonRequest{
		Reason: "employment ended",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if env.Repo.users[target].Status != users.StatusActive {
		t.Fatal("expected last superuser to remain active")
	}
}

func TestDeactivateUser_SelfDeactivation_Returns409(t *testing.T) {
	env := Setup(t)
	actorID, actorToken := env.NewActor(t, false, "users:deactivate")
	env.Repo.users[actorID] = users.User{ID: actorID, Status: users.StatusActive}

	rec := env.JSON(t, http.MethodPost, "/users/"+actorID+"/deactivate", actorToken, users.LifecycleReasonRequest{
		Reason: "test",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSuspendAndRestoreUserAreExplicitAndIdempotent(t *testing.T) {
	env := Setup(t)
	target := env.CreateUser(t, users.StatusActive)
	actorID, token := env.NewActor(t, false, "users:deactivate")

	suspended := env.JSON(t, http.MethodPost, "/users/"+target+"/suspend", token, users.LifecycleReasonRequest{Reason: "leave of absence"})
	if suspended.Code != http.StatusOK {
		t.Fatalf("suspend: %d %s", suspended.Code, suspended.Body.String())
	}
	var profile users.UserProfileResponse
	Decode(t, suspended, &profile)
	if profile.Status != "suspended" || profile.SuspendedAt == nil || profile.SuspendedBy == nil || *profile.SuspendedBy != actorID || profile.SuspensionReason == nil {
		t.Fatalf("missing suspension metadata: %+v", profile)
	}
	repeated := env.JSON(t, http.MethodPost, "/users/"+target+"/suspend", token, users.LifecycleReasonRequest{Reason: "different reason"})
	if repeated.Code != http.StatusOK {
		t.Fatalf("repeat suspend: %d %s", repeated.Code, repeated.Body.String())
	}

	restored := env.JSON(t, http.MethodPost, "/users/"+target+"/restore", token, users.LifecycleReasonRequest{Reason: "returned"})
	if restored.Code != http.StatusOK {
		t.Fatalf("restore: %d %s", restored.Code, restored.Body.String())
	}
	profile = users.UserProfileResponse{}
	Decode(t, restored, &profile)
	if profile.Status != "active" || profile.SuspendedAt != nil || profile.SuspensionReason != nil {
		t.Fatalf("suspension metadata was not cleared: %+v", profile)
	}
	repeated = env.JSON(t, http.MethodPost, "/users/"+target+"/restore", token, users.LifecycleReasonRequest{Reason: "returned"})
	if repeated.Code != http.StatusOK {
		t.Fatalf("repeat restore: %d %s", repeated.Code, repeated.Body.String())
	}
}

func TestUpdateUserCannotMutateLifecycleStatus(t *testing.T) {
	env := Setup(t)
	target := env.CreateUser(t, users.StatusActive)
	_, token := env.NewActor(t, false, "users:write")
	status := "suspended"
	rec := env.JSON(t, http.MethodPatch, "/users/"+target, token, users.UpdateUserRequest{Status: &status})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected explicit lifecycle endpoint requirement, got %d %s", rec.Code, rec.Body.String())
	}
	if env.Repo.users[target].Status != users.StatusActive {
		t.Fatal("generic update changed lifecycle status")
	}
}

func TestSuspendSuperuserRequiresSuperuserActor(t *testing.T) {
	env := Setup(t)
	target := env.CreateUser(t, users.StatusActive)
	env.Repo.superusers[target] = true
	other := env.CreateUser(t, users.StatusActive)
	env.Repo.superusers[other] = true
	_, token := env.NewActor(t, false, "users:deactivate")
	rec := env.JSON(t, http.MethodPost, "/users/"+target+"/suspend", token, users.LifecycleReasonRequest{Reason: "test"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected hierarchy denial, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestLastSuperuserCannotBeDemotedByRoleReplacement(t *testing.T) {
	env := Setup(t)
	target := env.CreateUser(t, users.StatusActive)
	env.Repo.superusers[target] = true
	env.CreateRole("ordinary", "Staff", false, false)
	_, token := env.NewActor(t, true, "users:write")
	rec := env.JSON(t, http.MethodPatch, "/users/"+target, token, users.UpdateUserRequest{RoleIDs: []string{"ordinary"}})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected last-superuser protection, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestSecurityStatusReturnsLifecycleMetadataWithoutSecrets(t *testing.T) {
	env := Setup(t)
	target := env.CreateUser(t, users.StatusActive)
	actorID, lifecycleToken := env.NewActor(t, false, "users:deactivate")
	rec := env.JSON(t, http.MethodPost, "/users/"+target+"/suspend", lifecycleToken, users.LifecycleReasonRequest{Reason: "security investigation"})
	if rec.Code != http.StatusOK {
		t.Fatalf("suspend: %d %s", rec.Code, rec.Body.String())
	}
	_, readToken := env.NewActor(t, false, "users:read")
	rec = env.JSON(t, http.MethodGet, "/users/"+target+"/security-status", readToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("security status: %d %s", rec.Code, rec.Body.String())
	}
	var status users.SecurityStatusResponse
	Decode(t, rec, &status)
	if status.Status != "suspended" || status.SuspendedBy == nil || *status.SuspendedBy != actorID || status.SuspensionReason == nil || *status.SuspensionReason != "security investigation" {
		t.Fatalf("unexpected status: %+v", status)
	}
}
