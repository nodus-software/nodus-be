package test_auth

import (
	"net/http"
	"testing"
)

func TestSessions_ListShowsCurrentSession(t *testing.T) {
	env := Setup(t)
	user := env.NewAuthedUser(t)

	rec := env.JSON(t, http.MethodGet, "/auth/sessions", user.AccessToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var sessions []struct {
		ID      string `json:"id"`
		Current bool   `json:"current"`
	}
	Decode(t, rec, &sessions)
	if len(sessions) != 1 {
		t.Fatalf("expected exactly one session, got %d", len(sessions))
	}
	if sessions[0].ID != user.SessionID || !sessions[0].Current {
		t.Fatalf("expected the current session to be flagged, got %+v", sessions[0])
	}
}

func TestSessions_RevokeSpecificSession(t *testing.T) {
	env := Setup(t)
	user := env.NewAuthedUser(t)
	otherSessionID := env.CreateSession(t, user.UserID)

	rec := env.JSON(t, http.MethodDelete, "/auth/sessions/"+otherSessionID, user.AccessToken, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	listRec := env.JSON(t, http.MethodGet, "/auth/sessions", user.AccessToken, nil)
	var sessions []struct {
		ID string `json:"id"`
	}
	Decode(t, listRec, &sessions)
	for _, s := range sessions {
		if s.ID == otherSessionID {
			t.Fatalf("expected session %s to be revoked and excluded from the list", otherSessionID)
		}
	}
}

func TestSessions_RevokeUnknownSession_Returns404(t *testing.T) {
	env := Setup(t)
	user := env.NewAuthedUser(t)

	rec := env.JSON(t, http.MethodDelete, "/auth/sessions/00000000-0000-0000-0000-000000000000", user.AccessToken, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}
