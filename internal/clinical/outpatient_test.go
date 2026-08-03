package clinical

import (
	"context"
	"errors"
	"testing"

	"nodus-health/internal/audit"
)

type noopAudit struct{}

func (noopAudit) Record(context.Context, audit.Entry) error { return nil }

type outpatientRepo struct {
	Repository
	active              *Visit
	created             *Visit
	encounter           *Encounter
	encounters          []Encounter
	diagnoses           []Diagnosis
	observations        []Observation
	completeVisitCalled bool
}

func (r *outpatientRepo) FindActiveOutpatientVisit(context.Context, string) (*Visit, error) {
	if r.active == nil {
		return nil, nil
	}
	return r.active, nil
}
func (r *outpatientRepo) CreateVisit(_ context.Context, v Visit) (*Visit, error) {
	r.created = &v
	return &v, nil
}
func (r *outpatientRepo) ApplyVisitRouting(context.Context, Visit) error { return nil }
func (r *outpatientRepo) GetVisit(context.Context, string) (*Visit, error) {
	if r.created != nil {
		return r.created, nil
	}
	return &Visit{ID: "visit", PatientID: "patient", VisitType: "outpatient", Status: "active"}, nil
}
func (r *outpatientRepo) GetEncounter(context.Context, string) (*Encounter, error) {
	return r.encounter, nil
}
func (r *outpatientRepo) CreateObservations(_ context.Context, x []Observation) ([]Observation, error) {
	r.observations = x
	return x, nil
}
func (r *outpatientRepo) ListEncounters(context.Context, string) ([]Encounter, error) {
	return r.encounters, nil
}
func (r *outpatientRepo) ListDiagnoses(context.Context, string) ([]Diagnosis, error) {
	return r.diagnoses, nil
}
func (r *outpatientRepo) CompleteVisit(context.Context, string) (*Visit, error) {
	r.completeVisitCalled = true
	return &Visit{ID: "visit", Status: "completed"}, nil
}

func TestOutpatientCheckInRejectsDuplicateWithoutOverride(t *testing.T) {
	r := &outpatientRepo{active: &Visit{ID: "existing", Status: "active", VisitType: "outpatient"}}
	s := NewService(r, noopAudit{})
	_, err := s.OutpatientCheckIn(context.Background(), "actor", OutpatientCheckInRequest{PatientID: "patient"})
	if !errors.Is(err, ErrActiveVisit) {
		t.Fatalf("expected active visit conflict, got %v", err)
	}
	if r.created != nil {
		t.Fatal("visit must not be created")
	}
}
func TestOutpatientOverrideRequiresReason(t *testing.T) {
	r := &outpatientRepo{active: &Visit{ID: "existing"}}
	s := NewService(r, noopAudit{})
	_, err := s.OutpatientCheckIn(context.Background(), "actor", OutpatientCheckInRequest{PatientID: "patient", Override: true})
	if !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("expected reason required, got %v", err)
	}
}
func TestOutpatientOverrideCreatesVisit(t *testing.T) {
	reason := "clinically separate attendance"
	r := &outpatientRepo{active: &Visit{ID: "existing"}}
	s := NewService(r, noopAudit{})
	v, err := s.OutpatientCheckIn(context.Background(), "actor", OutpatientCheckInRequest{PatientID: "patient", Override: true, OverrideReason: &reason})
	if err != nil {
		t.Fatal(err)
	}
	if v.VisitType != "outpatient" || r.created == nil {
		t.Fatalf("expected outpatient visit, got %#v", v)
	}
}
func TestRecordObservationsRejectsAmbiguousValue(t *testing.T) {
	r := &outpatientRepo{encounter: &Encounter{ID: "enc", VisitID: "visit", Status: "in_progress"}}
	s := NewService(r, noopAudit{})
	n := 37.2
	text := "high"
	_, err := s.RecordObservations(context.Background(), "actor", "enc", RecordObservationsRequest{Observations: []ObservationInput{{Code: "temperature", ValueNumeric: &n, ValueText: &text}}})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
	if len(r.observations) != 0 {
		t.Fatal("invalid observations must not persist")
	}
}
func TestCompleteTriageRequiresConsultationQueue(t *testing.T) {
	r := &outpatientRepo{encounter: &Encounter{ID: "enc", EncounterType: "triage", Status: "in_progress"}}
	s := NewService(r, noopAudit{})
	_, err := s.CompleteEncounter(context.Background(), "actor", "enc", CompleteEncounterRequest{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected consultation queue validation, got %v", err)
	}
}
func TestCompleteVisitRequiresCompletedConsultation(t *testing.T) {
	r := &outpatientRepo{encounters: []Encounter{{EncounterType: "triage", Status: "completed"}}}
	s := NewService(r, noopAudit{})
	_, err := s.CompleteVisit(context.Background(), "actor", "visit")
	if !errors.Is(err, ErrVisitIncomplete) {
		t.Fatalf("expected incomplete visit, got %v", err)
	}
	if r.completeVisitCalled {
		t.Fatal("incomplete visit must not be completed")
	}
}
func TestCompleteVisitAfterConsultation(t *testing.T) {
	r := &outpatientRepo{encounters: []Encounter{{EncounterType: "consultation", Status: "completed"}}, diagnoses: []Diagnosis{{Kind: "final"}}}
	s := NewService(r, noopAudit{})
	v, err := s.CompleteVisit(context.Background(), "actor", "visit")
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "completed" || !r.completeVisitCalled {
		t.Fatalf("expected completion, got %#v", v)
	}
}
func TestCompleteVisitRequiresFinalDiagnosis(t *testing.T) {
	r := &outpatientRepo{encounters: []Encounter{{EncounterType: "consultation", Status: "completed"}}}
	s := NewService(r, noopAudit{})
	_, err := s.CompleteVisit(context.Background(), "actor", "visit")
	if !errors.Is(err, ErrVisitIncomplete) {
		t.Fatalf("expected final diagnosis requirement, got %v", err)
	}
}
