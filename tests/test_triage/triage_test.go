package test_triage

import (
	"net/http"
	"testing"

	"nodus-health/internal/clinical"
)

func TestCallThenStartTriageReturnsPinnedForm(t *testing.T) {
	env := Setup(t)
	queueToken := env.NewActor(t, "queues:operate")
	triageToken := env.NewActor(t, "outpatient:triage")

	called := env.JSON(t, http.MethodPost, "/clinical/queue-entries/entry-1/transition", queueToken, clinical.TransitionRequest{Status: "called"})
	if called.Code != http.StatusOK {
		t.Fatalf("expected call to return 200, got %d: %s", called.Code, called.Body.String())
	}

	started := env.JSON(t, http.MethodPost, "/clinical/queue-entries/entry-1/start-triage", triageToken, nil)
	if started.Code != http.StatusCreated {
		t.Fatalf("expected triage start to return 201, got %d: %s", started.Code, started.Body.String())
	}
	var result clinical.TriageStart
	decodeData(t, started, &result)
	if result.QueueEntry.Status != "in_service" || result.Encounter.EncounterType != "triage" {
		t.Fatalf("unexpected started workflow: %+v", result)
	}
	if result.Form.TemplateVersion.Status != "published" || len(result.Form.TemplateVersion.Definition.Sections) == 0 {
		t.Fatalf("expected a pinned published form definition, got %+v", result.Form)
	}
}

func TestStartTriageRequiresTriagePermission(t *testing.T) {
	env := Setup(t)
	env.Repo.entry.Status = "called"
	queueOnly := env.NewActor(t, "queues:operate")

	response := env.JSON(t, http.MethodPost, "/clinical/queue-entries/entry-1/start-triage", queueOnly, nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", response.Code, response.Body.String())
	}
	if env.Repo.startCalls != 0 {
		t.Fatal("forbidden request must not start triage")
	}
}

func TestStartTriageRequiresCalledEntry(t *testing.T) {
	env := Setup(t)
	triageToken := env.NewActor(t, "outpatient:triage")

	response := env.JSON(t, http.MethodPost, "/clinical/queue-entries/entry-1/start-triage", triageToken, nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", response.Code, response.Body.String())
	}
}

func TestStartTriageRejectsNonTriageQueue(t *testing.T) {
	env := Setup(t)
	env.Repo.entry.Status = "called"
	env.Repo.queueKind = "consultation"
	triageToken := env.NewActor(t, "outpatient:triage")

	response := env.JSON(t, http.MethodPost, "/clinical/queue-entries/entry-1/start-triage", triageToken, nil)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", response.Code, response.Body.String())
	}
}

func TestCallThenStartConsultationReturnsPinnedForm(t *testing.T) {
	env := Setup(t)
	env.Repo.queueKind = "consultation"
	env.Repo.entry.ServicePointKind = "consultation"
	env.Repo.triageCompleted = true
	queueToken := env.NewActor(t, "queues:operate")
	consultToken := env.NewActor(t, "outpatient:consult")

	called := env.JSON(t, http.MethodPost, "/clinical/queue-entries/entry-1/transition", queueToken, clinical.TransitionRequest{Status: "called"})
	if called.Code != http.StatusOK {
		t.Fatalf("expected call to return 200, got %d: %s", called.Code, called.Body.String())
	}

	started := env.JSON(t, http.MethodPost, "/clinical/queue-entries/entry-1/start-consultation", consultToken, nil)
	if started.Code != http.StatusCreated {
		t.Fatalf("expected consultation start to return 201, got %d: %s", started.Code, started.Body.String())
	}
	var result clinical.EncounterStart
	decodeData(t, started, &result)
	if result.QueueEntry.Status != "in_service" || result.Encounter.EncounterType != "consultation" {
		t.Fatalf("unexpected started workflow: %+v", result)
	}
	if result.Form.TemplateVersion.Status != "published" || len(result.Form.TemplateVersion.Definition.Sections) == 0 {
		t.Fatalf("expected a pinned published form definition, got %+v", result.Form)
	}
}

func TestStartConsultationRequiresConsultPermission(t *testing.T) {
	env := Setup(t)
	env.Repo.queueKind = "consultation"
	env.Repo.entry.Status = "called"
	triageOnly := env.NewActor(t, "outpatient:triage", "queues:operate")

	response := env.JSON(t, http.MethodPost, "/clinical/queue-entries/entry-1/start-consultation", triageOnly, nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", response.Code, response.Body.String())
	}
	if env.Repo.startCalls != 0 {
		t.Fatal("forbidden request must not start consultation")
	}
}

func TestStartConsultationRequiresCompletedTriage(t *testing.T) {
	env := Setup(t)
	env.Repo.queueKind = "consultation"
	env.Repo.entry.Status = "called"
	consultToken := env.NewActor(t, "outpatient:consult")

	response := env.JSON(t, http.MethodPost, "/clinical/queue-entries/entry-1/start-consultation", consultToken, nil)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", response.Code, response.Body.String())
	}
}

// A bare transition into in_service is what stranded a patient with no
// consultation page: the entry left the waiting board without an encounter.
func TestClinicalQueueRejectsBareInServiceTransition(t *testing.T) {
	env := Setup(t)
	env.Repo.entry.Status = "called"
	queueToken := env.NewActor(t, "queues:operate")

	for _, kind := range []string{"triage", "consultation"} {
		env.Repo.entry.ServicePointKind = kind
		response := env.JSON(t, http.MethodPost, "/clinical/queue-entries/entry-1/transition", queueToken, clinical.TransitionRequest{Status: "in_service"})
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s queue: expected 422, got %d: %s", kind, response.Code, response.Body.String())
		}
		if env.Repo.entry.Status != "called" {
			t.Fatalf("%s queue: entry must stay called, got %s", kind, env.Repo.entry.Status)
		}
	}

	// Queues that are not staffed off an encounter keep the plain transition.
	env.Repo.entry.ServicePointKind = "laboratory"
	response := env.JSON(t, http.MethodPost, "/clinical/queue-entries/entry-1/transition", queueToken, clinical.TransitionRequest{Status: "in_service"})
	if response.Code != http.StatusOK {
		t.Fatalf("expected laboratory queue transition to return 200, got %d: %s", response.Code, response.Body.String())
	}
}

func TestConsultPermissionCannotModifyTriageForm(t *testing.T) {
	env := Setup(t)
	env.Repo.encounter = &clinical.Encounter{ID: "triage-encounter", VisitID: "visit-1", EncounterType: "triage", Status: "in_progress"}
	consultToken := env.NewActor(t, "outpatient:consult")

	response := env.JSON(t, http.MethodPut, "/clinical/encounters/triage-encounter/form", consultToken, clinical.SaveEncounterFormRequest{})
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", response.Code, response.Body.String())
	}
}
