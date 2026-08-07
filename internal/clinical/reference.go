package clinical

import (
	"context"
	"strings"

	"nodus-health/internal/audit"
	"nodus-health/pkg/utility"
)

var allergenCategories = map[string]bool{"medication": true, "food": true, "contrast": true, "contact": true, "insect": true, "environmental": true, "other": true}

func cleanAliases(xs []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, x := range xs {
		x = strings.TrimSpace(x)
		k := strings.ToLower(x)
		if x != "" && !seen[k] {
			seen[k] = true
			out = append(out, x)
		}
	}
	return out
}

func (s *Service) SearchICD11(c context.Context, q string) ([]Concept, error) {
	q = strings.TrimSpace(q)
	if len(q) < 2 {
		return []Concept{}, nil
	}
	return s.repo.SearchICD11Concepts(c, q, 20)
}
func (s *Service) ListDiagnosisConcepts(c context.Context, f DiagnosisFilters) (*DiagnosisConceptPage, error) {
	f.Query = strings.TrimSpace(f.Query)
	f.Chapter = strings.TrimSpace(f.Chapter)
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 50
	}
	if f.PageSize > 200 {
		f.PageSize = 200
	}
	if f.Availability != "" && f.Availability != "all" && f.Availability != "enabled" && f.Availability != "disabled" {
		return nil, ErrInvalidInput
	}
	x, total, e := s.repo.ListDiagnosisConcepts(c, f)
	return &DiagnosisConceptPage{Items: x, Total: total, Page: f.Page, PageSize: f.PageSize}, e
}
func (s *Service) SetDiagnosisConceptEnabled(c context.Context, actor, id string, enabled bool) (*Concept, error) {
	if id == "" {
		return nil, ErrInvalidInput
	}
	x, e := s.repo.SetDiagnosisConceptEnabled(c, id, enabled)
	if e == nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "diagnosis_configuration_updated", Result: audit.ResultSuccess, TargetResource: id, Metadata: map[string]any{"enabled": enabled}})
	}
	return x, e
}
func (s *Service) ListAllergens(c context.Context) ([]Allergen, error) {
	return s.repo.ListAllergens(c)
}
func validateAllergen(code, name, category string) bool {
	return strings.TrimSpace(code) != "" && strings.TrimSpace(name) != "" && allergenCategories[category]
}
func (s *Service) CreateAllergen(c context.Context, actor string, q CreateAllergenRequest) (*Allergen, error) {
	q.Code = strings.ToLower(strings.TrimSpace(q.Code))
	q.Name = strings.TrimSpace(q.Name)
	q.Category = strings.ToLower(strings.TrimSpace(q.Category))
	q.Aliases = cleanAliases(q.Aliases)
	if !validateAllergen(q.Code, q.Name, q.Category) {
		return nil, ErrInvalidInput
	}
	id, e := utility.GenerateUUID()
	if e != nil {
		return nil, e
	}
	x, e := s.repo.CreateAllergen(c, Allergen{ID: id, Code: q.Code, Name: q.Name, Category: q.Category, Aliases: q.Aliases, Active: true})
	if e == nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "allergen_configuration_created", Result: audit.ResultSuccess, TargetResource: id})
	}
	return x, e
}
func (s *Service) UpdateAllergen(c context.Context, actor, id string, q UpdateAllergenRequest) (*Allergen, error) {
	if id == "" || (q.Code == nil && q.Name == nil && q.Category == nil && q.Aliases == nil) {
		return nil, ErrInvalidInput
	}
	if q.Code != nil {
		v := strings.ToLower(strings.TrimSpace(*q.Code))
		if v == "" {
			return nil, ErrInvalidInput
		}
		q.Code = &v
	}
	if q.Name != nil {
		v := strings.TrimSpace(*q.Name)
		if v == "" {
			return nil, ErrInvalidInput
		}
		q.Name = &v
	}
	if q.Category != nil {
		v := strings.ToLower(strings.TrimSpace(*q.Category))
		if !allergenCategories[v] {
			return nil, ErrInvalidInput
		}
		q.Category = &v
	}
	if q.Aliases != nil {
		v := cleanAliases(*q.Aliases)
		q.Aliases = &v
	}
	x, e := s.repo.UpdateAllergen(c, id, q)
	if e == nil && s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: "allergen_configuration_updated", Result: audit.ResultSuccess, TargetResource: id})
	}
	return x, e
}
func (s *Service) SetAllergenActive(c context.Context, actor, id string, active bool) (*Allergen, error) {
	if id == "" {
		return nil, ErrInvalidInput
	}
	x, e := s.repo.SetAllergenActive(c, id, active)
	if e == nil && s.audit != nil {
		action := "allergen_configuration_deactivated"
		if active {
			action = "allergen_configuration_reactivated"
		}
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: action, Result: audit.ResultSuccess, TargetResource: id})
	}
	return x, e
}
