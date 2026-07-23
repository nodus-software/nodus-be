package test_roles

import (
	"net/http"
	"testing"

	"nodus-health/internal/roles"
)

func TestListRoles_ReturnsCreatedRoles(t *testing.T) {
	env := Setup(t)
	env.SeedPermission("patients:read")
	_, actorToken := env.NewUser(t, true, "roles:read", "roles:write")

	createRec := env.JSON(t, http.MethodPost, "/roles", actorToken, roles.CreateRoleRequest{
		Name: "Nurse", Permissions: []string{"patients:read"},
	})
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	listRec := env.JSON(t, http.MethodGet, "/roles", actorToken, nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	var list []roles.RoleResponse
	Decode(t, listRec, &list)
	if len(list) != 1 || list[0].Name != "Nurse" {
		t.Fatalf("expected one role named Nurse, got %+v", list)
	}
	if len(list[0].Permissions) != 1 || list[0].Permissions[0] != "patients:read" {
		t.Fatalf("expected permissions=[patients:read], got %v", list[0].Permissions)
	}
}

func TestCreateRole_RequiresSuperuser(t *testing.T) {
	env := Setup(t)
	env.SeedPermission("patients:read")
	_, actorToken := env.NewUser(t, false, "roles:write")

	rec := env.JSON(t, http.MethodPost, "/roles", actorToken, roles.CreateRoleRequest{
		Name: "Nurse", Permissions: []string{"patients:read"},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-superuser, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateRole_UnknownPermission_Returns422(t *testing.T) {
	env := Setup(t)
	_, actorToken := env.NewUser(t, true, "roles:write")

	rec := env.JSON(t, http.MethodPost, "/roles", actorToken, roles.CreateRoleRequest{
		Name: "Ghost Role", Permissions: []string{"does-not-exist"},
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateRole_DuplicateName_Returns409(t *testing.T) {
	env := Setup(t)
	env.SeedPermission("patients:read")
	_, actorToken := env.NewUser(t, true, "roles:write")

	req := roles.CreateRoleRequest{Name: "Nurse", Permissions: []string{"patients:read"}}
	first := env.JSON(t, http.MethodPost, "/roles", actorToken, req)
	if first.Code != http.StatusCreated {
		t.Fatalf("expected first create to succeed, got %d: %s", first.Code, first.Body.String())
	}

	second := env.JSON(t, http.MethodPost, "/roles", actorToken, req)
	if second.Code != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate name, got %d: %s", second.Code, second.Body.String())
	}
}

func TestRoutesRequireAuthentication(t *testing.T) {
	env := Setup(t)
	rec := env.JSON(t, http.MethodGet, "/roles", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a token, got %d: %s", rec.Code, rec.Body.String())
	}
}
