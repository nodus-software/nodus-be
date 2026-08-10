package clinical

import (
	"context"
	"strings"
	"time"

	"nodus-health/internal/audit"
	"nodus-health/pkg/utility"
)

func (s *Service) OutpatientCheckIn(c context.Context, actor string, q OutpatientCheckInRequest) (*Visit, error) {
	if strings.TrimSpace(q.PatientID) == "" {
		return nil, ErrInvalidInput
	}
	active, err := s.repo.FindActiveOutpatientVisit(c, q.PatientID)
	if err != nil {
		return nil, err
	}
	if active != nil {
		if !q.Override {
			entry, _ := s.repo.GetCurrentVisitQueueEntry(c, active.ID)
			return nil, &ActiveVisitConflictError{Visit: *active, QueueEntry: entry}
		}
		if q.OverrideReason == nil || strings.TrimSpace(*q.OverrideReason) == "" {
			return nil, ErrReasonRequired
		}
	}
	v, err := s.CreateVisit(c, actor, CreateVisitRequest{PatientID: q.PatientID, VisitType: "outpatient", Reason: q.Reason, ServicePointID: q.ServicePointID})
	if err == nil && active != nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "outpatient_duplicate_visit_overridden", Result: audit.ResultSuccess, TargetResource: v.ID, Metadata: map[string]any{"existing_visit_id": active.ID, "reason": strings.TrimSpace(*q.OverrideReason)}})
	}
	return v, err
}

func (s *Service) ListOutpatientVisits(c context.Context, f OutpatientVisitFilters) ([]OutpatientVisitListItem, int, error) {
	if f.Status != "" && !map[string]bool{"active": true, "completed": true, "cancelled": true}[f.Status] {
		return nil, 0, ErrInvalidInput
	}
	if f.Stage != "" && !map[string]bool{"checked_in": true, "triage": true, "consultation": true, "completed": true, "cancelled": true}[f.Stage] {
		return nil, 0, ErrInvalidInput
	}
	return s.repo.ListOutpatientVisitItems(c, f)
}
func (s *Service) ListEncounters(c context.Context, visitID string) ([]Encounter, error) {
	if visitID == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.ListEncounters(c, visitID)
}
func (s *Service) GetEncounter(c context.Context, encounterID string) (*Encounter, error) {
	if strings.TrimSpace(encounterID) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetEncounter(c, encounterID)
}
func (s *Service) ListAllergies(c context.Context, patientID string) ([]Allergy, error) {
	if patientID == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.ListAllergies(c, patientID)
}

func (s *Service) CreateEncounter(c context.Context, actor, visitID string, q CreateEncounterRequest) (*Encounter, error) {
	if !map[string]bool{"triage": true, "consultation": true}[q.EncounterType] {
		return nil, ErrInvalidInput
	}
	v, err := s.repo.GetVisit(c, visitID)
	if err != nil {
		return nil, err
	}
	if v.VisitType != "outpatient" || v.Status != "active" {
		return nil, ErrInvalidTransition
	}
	if q.EncounterType == "consultation" {
		encounters, e := s.repo.ListEncounters(c, visitID)
		if e != nil {
			return nil, e
		}
		triageComplete := false
		for _, item := range encounters {
			if item.EncounterType == "triage" && item.Status == "completed" {
				triageComplete = true
			}
		}
		if !triageComplete {
			if !q.Override {
				return nil, ErrInvalidTransition
			}
			if strings.TrimSpace(value(q.OverrideReason)) == "" {
				return nil, ErrReasonRequired
			}
		}
	}
	id, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	formID, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	x, err := s.repo.CreateEncounterWithForm(c, Encounter{ID: id, VisitID: visitID, EncounterType: q.EncounterType, Status: "in_progress", ServicePointID: q.ServicePointID, ClinicianID: &actor, StartedAt: &now}, formID, actor)
	if err == nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_encounter_started", Result: audit.ResultSuccess, TargetResource: id, Metadata: map[string]any{"type": q.EncounterType, "visit_id": visitID}})
	}
	return x, err
}

func (s *Service) RecordObservations(c context.Context, actor, encounterID string, q RecordObservationsRequest) ([]Observation, error) {
	if len(q.Observations) == 0 || len(q.Observations) > 50 {
		return nil, ErrInvalidInput
	}
	e, err := s.repo.GetEncounter(c, encounterID)
	if err != nil {
		return nil, err
	}
	if e.Status != "in_progress" {
		return nil, ErrInvalidTransition
	}
	if e.EncounterType != "triage" {
		return nil, ErrInvalidInput
	}
	v, err := s.repo.GetVisit(c, e.VisitID)
	if err != nil {
		return nil, err
	}
	out := make([]Observation, 0, len(q.Observations))
	for _, in := range q.Observations {
		if strings.TrimSpace(in.Code) == "" || (in.ValueNumeric == nil) == (in.ValueText == nil) {
			return nil, ErrInvalidInput
		}
		id, xerr := utility.GenerateUUID()
		if xerr != nil {
			return nil, xerr
		}
		out = append(out, Observation{ID: id, PatientID: v.PatientID, VisitID: v.ID, EncounterID: &e.ID, Code: strings.TrimSpace(in.Code), ValueNumeric: in.ValueNumeric, ValueText: in.ValueText, Unit: in.Unit, RecordedBy: actor, ObservedAt: time.Now().UTC()})
	}
	x, err := s.repo.CreateObservations(c, out)
	if err == nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_observations_recorded", Result: audit.ResultSuccess, TargetResource: encounterID, Metadata: map[string]any{"count": len(out)}})
	}
	return x, err
}

func (s *Service) CompleteEncounter(c context.Context, actor, id string, q CompleteEncounterRequest) (*Encounter, error) {
	e, err := s.repo.GetEncounter(c, id)
	if err != nil {
		return nil, err
	}
	if e.Status != "in_progress" {
		return nil, ErrInvalidTransition
	}
	if q.ExpectedUpdatedAt != nil && !e.UpdatedAt.Equal(*q.ExpectedUpdatedAt) {
		return nil, ErrConflict
	}
	form, err := s.repo.GetEncounterForm(c, id)
	if err != nil {
		return nil, err
	}
	if form.Status != "submitted" {
		return nil, ErrFormIncomplete
	}
	x, err := s.repo.CompleteEncounter(c, id, actor)
	if err == nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_encounter_completed", Result: audit.ResultSuccess, TargetResource: id})
	}
	return x, err
}

func (s *Service) CreateNote(c context.Context, actor, visitID string, q CreateNoteRequest) (*ClinicalNote, error) {
	if strings.TrimSpace(q.NoteType) == "" || strings.TrimSpace(q.Body) == "" {
		return nil, ErrInvalidInput
	}
	v, err := s.repo.GetVisit(c, visitID)
	if err != nil {
		return nil, err
	}
	if v.Status != "active" {
		return nil, ErrInvalidTransition
	}
	if q.EncounterID != nil {
		e, eerr := s.repo.GetEncounter(c, *q.EncounterID)
		if eerr != nil {
			return nil, eerr
		}
		if e.VisitID != visitID {
			return nil, ErrInvalidInput
		}
	}
	id, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	return s.repo.CreateNote(c, ClinicalNote{ID: id, PatientID: v.PatientID, VisitID: v.ID, EncounterID: q.EncounterID, NoteType: strings.TrimSpace(q.NoteType), Body: strings.TrimSpace(q.Body), AuthoredBy: actor})
}
func (s *Service) CreateDiagnosis(c context.Context, actor, visitID string, q CreateDiagnosisRequest) (*Diagnosis, error) {
	if (strings.TrimSpace(q.Code) == "" && strings.TrimSpace(q.ConceptID) == "") || !map[string]bool{"provisional": true, "final": true, "secondary": true}[q.Kind] {
		return nil, ErrInvalidInput
	}
	v, err := s.repo.GetVisit(c, visitID)
	if err != nil {
		return nil, err
	}
	if v.Status != "active" {
		return nil, ErrInvalidTransition
	}
	if q.EncounterID != nil {
		e, eerr := s.repo.GetEncounter(c, *q.EncounterID)
		if eerr != nil {
			return nil, eerr
		}
		if e.VisitID != visitID {
			return nil, ErrInvalidInput
		}
	}
	id, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	concept, err := s.repo.ResolveICD11Concept(c, strings.TrimSpace(q.ConceptID), strings.TrimSpace(q.Code))
	if err != nil {
		return nil, err
	}
	return s.repo.CreateDiagnosis(c, Diagnosis{ID: id, PatientID: v.PatientID, VisitID: v.ID, EncounterID: q.EncounterID, ConceptID: concept.ID, Code: concept.Code, Display: concept.Display, Kind: q.Kind, Note: q.Note, RecordedBy: actor})
}
func (s *Service) CreateAllergy(c context.Context, actor, patientID string, q CreateAllergyRequest) (*Allergy, error) {
	legacy := strings.TrimSpace(q.Allergen)
	other := strings.TrimSpace(q.OtherAllergen)
	if legacy != "" && other == "" {
		other = legacy
	}
	if patientID == "" || (strings.TrimSpace(q.AllergenID) == "") == (other == "") {
		return nil, ErrInvalidInput
	}
	if q.Severity != nil && !map[string]bool{"mild": true, "moderate": true, "severe": true}[*q.Severity] {
		return nil, ErrInvalidInput
	}
	id, err := utility.GenerateUUID()
	if err != nil {
		return nil, err
	}
	x := Allergy{ID: id, PatientID: patientID, Allergen: other, Reaction: q.Reaction, Severity: q.Severity, Status: "active", RecordedBy: actor, IsCustom: true}
	if q.AllergenID != "" {
		a, e := s.repo.GetActiveAllergen(c, q.AllergenID)
		if e != nil {
			return nil, e
		}
		x.AllergenID = &a.ID
		x.Allergen = a.Name
		x.AllergenCode = &a.Code
		x.Category = &a.Category
		x.IsCustom = false
	}
	created, e := s.repo.CreateAllergy(c, x)
	if e == nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_allergy_recorded", Result: audit.ResultSuccess, TargetResource: id, Metadata: map[string]any{"patient_id": patientID, "allergen_id": x.AllergenID, "custom": x.IsCustom}})
	}
	return created, e
}
func (s *Service) CompleteVisit(c context.Context, actor, id string, q CompleteVisitRequest) (*Visit, error) {
	v, err := s.repo.GetVisit(c, id)
	if err != nil {
		return nil, err
	}
	if v.Status != "active" {
		return nil, ErrInvalidTransition
	}
	if q.ExpectedUpdatedAt != nil && !v.UpdatedAt.Equal(*q.ExpectedUpdatedAt) {
		return nil, ErrConflict
	}
	enc, err := s.repo.ListEncounters(c, id)
	if err != nil {
		return nil, err
	}
	hasConsult := false
	for _, e := range enc {
		if e.EncounterType == "consultation" && e.Status == "completed" {
			hasConsult = true
		}
	}
	if !hasConsult {
		return nil, ErrVisitIncomplete
	}
	diagnoses, err := s.repo.ListDiagnoses(c, id)
	if err != nil {
		return nil, err
	}
	hasFinal := false
	for _, diagnosis := range diagnoses {
		if diagnosis.Kind == "final" {
			hasFinal = true
			break
		}
	}
	if !hasFinal {
		return nil, ErrVisitIncomplete
	}
	pending, err := s.repo.HasUnresolvedReviewOrders(c, id)
	if err != nil {
		return nil, err
	}
	if pending {
		if !q.Override {
			return nil, ErrVisitIncomplete
		}
		if strings.TrimSpace(value(q.OverrideReason)) == "" {
			return nil, ErrReasonRequired
		}
	}
	x, err := s.repo.CompleteVisit(c, id)
	if err == nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_visit_completed", Result: audit.ResultSuccess, TargetResource: id, Metadata: map[string]any{"order_override": pending, "reason": value(q.OverrideReason)}})
	}
	return x, err
}

func (s *Service) OutpatientVisitContext(c context.Context, id string) (*OutpatientVisitContext, error) {
	v, e := s.repo.GetVisit(c, id)
	if e != nil {
		return nil, e
	}
	enc, e := s.repo.ListEncounters(c, id)
	if e != nil {
		return nil, e
	}
	all, e := s.repo.ListAllergies(c, v.PatientID)
	if e != nil {
		return nil, e
	}
	orders, _, e := s.repo.ListOrders(c, OrderFilters{VisitID: id, Page: 1, PerPage: 100})
	if e != nil {
		return nil, e
	}
	entry, _ := s.repo.GetCurrentVisitQueueEntry(c, id)
	stage := "checked_in"
	if v.Status == "completed" || v.Status == "cancelled" {
		stage = v.Status
	} else {
		for _, x := range enc {
			if x.EncounterType == "triage" {
				stage = "triage"
			}
			if x.EncounterType == "consultation" {
				stage = "consultation"
			}
		}
	}
	return &OutpatientVisitContext{Visit: *v, CurrentQueueEntry: entry, Encounters: enc, Allergies: all, Orders: orders, WorkflowStage: stage}, nil
}
func (s *Service) VisitSummary(c context.Context, id string) (*VisitSummary, error) {
	v, e := s.repo.GetVisit(c, id)
	if e != nil {
		return nil, e
	}
	enc, e := s.repo.ListEncounters(c, id)
	if e != nil {
		return nil, e
	}
	obs, e := s.repo.ListObservations(c, id)
	if e != nil {
		return nil, e
	}
	notes, e := s.repo.ListNotes(c, id)
	if e != nil {
		return nil, e
	}
	dx, e := s.repo.ListDiagnoses(c, id)
	if e != nil {
		return nil, e
	}
	all, e := s.repo.ListAllergies(c, v.PatientID)
	if e != nil {
		return nil, e
	}
	forms, e := s.repo.ListEncounterForms(c, id)
	if e != nil {
		return nil, e
	}
	return &VisitSummary{Visit: *v, Encounters: enc, Observations: obs, Notes: notes, Diagnoses: dx, Allergies: all, Forms: forms}, nil
}
