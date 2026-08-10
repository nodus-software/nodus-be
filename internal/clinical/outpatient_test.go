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
	allergen            *Allergen
	allergy             *Allergy
	form                *EncounterForm
	formCreated         bool
	completeVisitCalled bool
}

func (r *outpatientRepo) GetActiveAllergen(context.Context, string) (*Allergen, error) {
	if r.allergen == nil {
		return nil, ErrInvalidInput
	}
	return r.allergen, nil
}
func (r *outpatientRepo) CreateAllergy(_ context.Context, x Allergy) (*Allergy, error) {
	r.allergy = &x
	return &x, nil
}

func (r *outpatientRepo) FindActiveOutpatientVisit(context.Context, string) (*Visit, error) {
	if r.active == nil {
		return nil, nil
	}
	return r.active, nil
}
func (r *outpatientRepo) GetCurrentVisitQueueEntry(context.Context, string) (*QueueEntry, error) {
	return nil, ErrNotFound
}
func (r *outpatientRepo) HasUnresolvedReviewOrders(context.Context, string) (bool, error) {
	return false, nil
}
func (r *outpatientRepo) ApplyEventRouting(context.Context, string, string, string, string, string, string, *string) error {
	return nil
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
func (r *outpatientRepo) CompleteEncounter(_ context.Context, id, actor string) (*Encounter, error) {
	r.encounter.Status = "completed"
	return r.encounter, nil
}
func (r *outpatientRepo) GetEncounterForm(context.Context, string) (*EncounterForm, error) {
	if r.form != nil {
		return r.form, nil
	}
	return &EncounterForm{Status: "submitted"}, nil
}
func (r *outpatientRepo) CreateEncounterWithForm(_ context.Context, x Encounter, formID, actor string) (*Encounter, error) {
	r.encounter = &x
	r.formCreated = true
	return &x, nil
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

func TestCreateAllergyUsesConfiguredSnapshot(t *testing.T) {
	r := &outpatientRepo{allergen: &Allergen{ID: "allergen-1", Code: "penicillins", Name: "Penicillins", Category: "medication", Active: true}}
	s := NewService(r, noopAudit{})
	x, e := s.CreateAllergy(context.Background(), "actor", "patient", CreateAllergyRequest{AllergenID: "allergen-1"})
	if e != nil {
		t.Fatal(e)
	}
	if x.Allergen != "Penicillins" || x.AllergenID == nil || x.IsCustom {
		t.Fatalf("unexpected allergy: %#v", x)
	}
}
func TestCreateAllergyRequiresCatalogueOrOtherButNotBoth(t *testing.T) {
	s := NewService(&outpatientRepo{}, noopAudit{})
	for _, q := range []CreateAllergyRequest{{}, {AllergenID: "one", OtherAllergen: "custom"}} {
		if _, e := s.CreateAllergy(context.Background(), "actor", "patient", q); !errors.Is(e, ErrInvalidInput) {
			t.Fatalf("expected invalid input for %#v, got %v", q, e)
		}
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
func TestCompleteTriageUsesConfiguredRouting(t *testing.T) {
	r := &outpatientRepo{encounter: &Encounter{ID: "enc", EncounterType: "triage", Status: "in_progress"}}
	s := NewService(r, noopAudit{})
	_, err := s.CompleteEncounter(context.Background(), "actor", "enc", CompleteEncounterRequest{})
	if err != nil {
		t.Fatalf("expected configured routing, got %v", err)
	}
}
func TestCompleteEncounterRequiresSubmittedTemplate(t *testing.T) {
	r := &outpatientRepo{encounter: &Encounter{ID: "enc", EncounterType: "consultation", Status: "in_progress"}, form: &EncounterForm{Status: "draft"}}
	s := NewService(r, noopAudit{})
	_, err := s.CompleteEncounter(context.Background(), "actor", "enc", CompleteEncounterRequest{})
	if !errors.Is(err, ErrFormIncomplete) {
		t.Fatalf("expected incomplete form validation, got %v", err)
	}
}
func TestCreateEncounterPinsDefaultTemplateForm(t *testing.T) {
	r := &outpatientRepo{encounters: []Encounter{{EncounterType: "triage", Status: "completed"}}}
	s := NewService(r, noopAudit{})
	x, err := s.CreateEncounter(context.Background(), "actor", "visit", CreateEncounterRequest{EncounterType: "consultation"})
	if err != nil {
		t.Fatal(err)
	}
	if x == nil || !r.formCreated {
		t.Fatalf("expected encounter and form, got encounter=%#v form=%v", x, r.formCreated)
	}
}
func TestCompleteVisitRequiresCompletedConsultation(t *testing.T) {
	r := &outpatientRepo{encounters: []Encounter{{EncounterType: "triage", Status: "completed"}}}
	s := NewService(r, noopAudit{})
	_, err := s.CompleteVisit(context.Background(), "actor", "visit", CompleteVisitRequest{})
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
	v, err := s.CompleteVisit(context.Background(), "actor", "visit", CompleteVisitRequest{})
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
	_, err := s.CompleteVisit(context.Background(), "actor", "visit", CompleteVisitRequest{})
	if !errors.Is(err, ErrVisitIncomplete) {
		t.Fatalf("expected final diagnosis requirement, got %v", err)
	}
}
