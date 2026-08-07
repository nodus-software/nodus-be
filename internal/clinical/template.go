package clinical

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"nodus-health/internal/audit"
	"nodus-health/pkg/utility"
)

func validTemplateEncounterType(v string) bool { return v == "triage" || v == "consultation" }

func (s *Service) ListTemplates(c context.Context, kind, status string) ([]ClinicalTemplate, error) {
	if kind != "" && !validTemplateEncounterType(kind) || status != "" && status != "active" && status != "archived" {
		return nil, ErrInvalidInput
	}
	return s.repo.ListTemplates(c, kind, status)
}
func (s *Service) GetTemplate(c context.Context, id string) (*ClinicalTemplate, error) {
	if id == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetTemplate(c, id)
}
func (s *Service) CreateTemplate(c context.Context, actor string, q CreateTemplateRequest) (*ClinicalTemplate, error) {
	q.Code = strings.ToLower(strings.TrimSpace(q.Code))
	q.Name = strings.TrimSpace(q.Name)
	if q.Code == "" || q.Name == "" || !validTemplateEncounterType(q.EncounterType) || ValidateTemplateDefinition(q.Definition) != nil {
		return nil, ErrInvalidInput
	}
	id, e := utility.GenerateUUID()
	if e != nil {
		return nil, e
	}
	vid, e := utility.GenerateUUID()
	if e != nil {
		return nil, e
	}
	x, e := s.repo.CreateTemplate(c, ClinicalTemplate{ID: id, Code: q.Code, Name: q.Name, Description: q.Description, EncounterType: q.EncounterType}, ClinicalTemplateVersion{ID: vid, TemplateID: id, Version: 1, Status: "draft", Definition: q.Definition, CreatedBy: &actor})
	if e == nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_template_created", Result: audit.ResultSuccess, TargetResource: id, Metadata: map[string]any{"encounter_type": q.EncounterType}})
	}
	return x, e
}
func (s *Service) CreateTemplateDraft(c context.Context, actor, id string) (*ClinicalTemplate, error) {
	vid, e := utility.GenerateUUID()
	if e != nil {
		return nil, e
	}
	return s.repo.CreateTemplateDraft(c, id, vid, actor)
}
func (s *Service) UpdateTemplateDraft(c context.Context, actor, id string, q UpdateTemplateDraftRequest) (*ClinicalTemplate, error) {
	if id == "" || ValidateTemplateDefinition(q.Definition) != nil {
		return nil, ErrInvalidInput
	}
	if q.Name != nil {
		v := strings.TrimSpace(*q.Name)
		if v == "" {
			return nil, ErrInvalidInput
		}
		q.Name = &v
	}
	return s.repo.UpdateTemplateDraft(c, id, actor, q)
}
func (s *Service) PublishTemplate(c context.Context, actor, id string) (*ClinicalTemplate, error) {
	x, e := s.repo.GetTemplate(c, id)
	if e != nil {
		return nil, e
	}
	if x.Draft == nil || ValidateTemplateDefinition(x.Draft.Definition) != nil {
		return nil, ErrInvalidInput
	}
	x, e = s.repo.PublishTemplate(c, id, actor)
	if e == nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_template_published", Result: audit.ResultSuccess, TargetResource: id})
	}
	return x, e
}
func (s *Service) SetDefaultTemplate(c context.Context, actor, id string) (*ClinicalTemplate, error) {
	x, e := s.repo.SetDefaultTemplate(c, id)
	if e == nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_template_default_changed", Result: audit.ResultSuccess, TargetResource: id})
	}
	return x, e
}
func (s *Service) ArchiveTemplate(c context.Context, actor, id, reason string) (*ClinicalTemplate, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, ErrReasonRequired
	}
	x, e := s.repo.ArchiveTemplate(c, id)
	if e == nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_template_archived", Result: audit.ResultSuccess, TargetResource: id, Metadata: map[string]any{"reason": strings.TrimSpace(reason)}})
	}
	return x, e
}

func (s *Service) GetEncounterForm(c context.Context, encounterID string) (*EncounterForm, error) {
	if encounterID == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetEncounterForm(c, encounterID)
}
func (s *Service) SaveEncounterForm(c context.Context, actor, encounterID string, q SaveEncounterFormRequest) (*EncounterForm, error) {
	if q.Answers == nil {
		return nil, ErrInvalidInput
	}
	f, e := s.repo.GetEncounterForm(c, encounterID)
	if e != nil {
		return nil, e
	}
	if f.Status != "draft" {
		return nil, ErrInvalidTransition
	}
	if errs := validateFormAnswers(f.TemplateVersion.Definition, q.Answers, false); len(errs) > 0 {
		return nil, &FormValidationError{Errors: errs}
	}
	return s.repo.SaveEncounterForm(c, encounterID, actor, q)
}
func (s *Service) SubmitEncounterForm(c context.Context, actor, encounterID string, q SubmitEncounterFormRequest) (*EncounterForm, error) {
	f, e := s.repo.GetEncounterForm(c, encounterID)
	if e != nil {
		return nil, e
	}
	if f.Status != "draft" {
		return nil, ErrInvalidTransition
	}
	if q.Revision != f.Revision {
		return nil, ErrConflict
	}
	if errs := validateFormAnswers(f.TemplateVersion.Definition, f.Answers, true); len(errs) > 0 {
		return nil, &FormValidationError{Errors: errs}
	}
	enc, e := s.repo.GetEncounter(c, encounterID)
	if e != nil {
		return nil, e
	}
	if enc.Status != "in_progress" {
		return nil, ErrInvalidTransition
	}
	visit, e := s.repo.GetVisit(c, enc.VisitID)
	if e != nil {
		return nil, e
	}
	obs := []Observation{}
	now := time.Now().UTC()
	for _, section := range f.TemplateVersion.Definition.Sections {
		for _, field := range section.Fields {
			if field.Binding == nil {
				continue
			}
			raw, ok := f.Answers[field.Key]
			if !ok {
				continue
			}
			id, e := utility.GenerateUUID()
			if e != nil {
				return nil, e
			}
			key := field.Key
			x := Observation{ID: id, PatientID: visit.PatientID, VisitID: visit.ID, EncounterID: &enc.ID, Code: field.Binding.Code, Unit: field.Binding.Unit, RecordedBy: actor, ObservedAt: now, SourceFormID: &f.ID, SourceFormFieldKey: &key}
			if field.Type == "number" {
				var v float64
				_ = json.Unmarshal(raw, &v)
				x.ValueNumeric = &v
			} else {
				var v string
				_ = json.Unmarshal(raw, &v)
				x.ValueText = &v
			}
			obs = append(obs, x)
		}
	}
	x, e := s.repo.SubmitEncounterForm(c, *f, actor, obs)
	if e == nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "clinical_encounter_form_submitted", Result: audit.ResultSuccess, TargetResource: f.ID, Metadata: map[string]any{"encounter_id": encounterID, "template_version_id": f.TemplateVersion.ID}})
	}
	return x, e
}
