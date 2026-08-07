package clinical

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type catalogueRepo struct {
	Repository
	service    *CatalogueService
	medication *MedicationDefinition
	existing   map[string]bool
	saved      *CatalogueImport
}

func (r *catalogueRepo) CreateCatalogueService(_ context.Context, x CatalogueService) (*CatalogueService, error) {
	r.service = &x
	return &x, nil
}
func (r *catalogueRepo) CreateMedicationDefinition(_ context.Context, x MedicationDefinition) (*MedicationDefinition, error) {
	r.medication = &x
	return &x, nil
}
func (r *catalogueRepo) CatalogueCodes(context.Context, string, []string) (map[string]bool, error) {
	return r.existing, nil
}

// A facility that seeded the prescribing vocabularies and left them as-is.
func (r *catalogueRepo) VocabularyCodes(_ context.Context, kind string) (map[string]string, error) {
	var entries []VocabularyEntry
	switch kind {
	case DosageFormKind:
		entries = DefaultDosageForms
	case RouteKind:
		entries = DefaultRoutes
	case UnitOfMeasureKind:
		entries = DefaultUnitsOfMeasure
	}
	out := map[string]string{}
	for _, e := range entries {
		out[e.Code] = e.Code
	}
	return out, nil
}
func (r *catalogueRepo) SaveCatalogueImport(_ context.Context, x CatalogueImport, _ string) error {
	r.saved = &x
	return nil
}
func (r *catalogueRepo) ResolveDepartmentCodes(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (r *catalogueRepo) ResolveServicePointCodes(context.Context, []string) (map[string]ServicePointImportReference, error) {
	return map[string]ServicePointImportReference{}, nil
}
func (r *catalogueRepo) ResolveCatalogueReferenceCodes(_ context.Context, _ string, keys []string) (map[string]string, error) {
	out := map[string]string{}
	for _, key := range keys {
		out[key] = "reference-id"
	}
	return out, nil
}

func TestCreateCatalogueServiceValidatesCategoryAndMoney(t *testing.T) {
	r := &catalogueRepo{}
	s := NewService(r, noopAudit{})
	price := -1.0
	_, err := s.CreateCatalogueService(context.Background(), "actor", ServiceCatalogueInput{Code: "CBC", Name: "Full blood count", Category: "laboratory", Price: &price})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid price, got %v", err)
	}
	_, err = s.CreateCatalogueService(context.Background(), "actor", ServiceCatalogueInput{Code: "CBC", Name: "Full blood count", Category: "unknown"})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid category, got %v", err)
	}
}

func TestMedicationRequiresStableLocalCode(t *testing.T) {
	r := &catalogueRepo{}
	s := NewService(r, noopAudit{})
	_, err := s.CreateMedicationDefinition(context.Background(), "actor", MedicationCatalogueInput{GenericName: "Amoxicillin", PrescriptionRequired: true})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected local code validation, got %v", err)
	}
}

func TestMedicationVocabularyIsResolvedAgainstConfiguredLists(t *testing.T) {
	r := &catalogueRepo{}
	s := NewService(r, noopAudit{})
	// Abbreviations clinicians actually type resolve to the configured code.
	_, err := s.CreateMedicationDefinition(context.Background(), "actor", MedicationCatalogueInput{
		Code: "MED-1", GenericName: "Amoxicillin", DosageForm: ptr("Tab"), Route: ptr("PO"), UnitOfMeasure: ptr("Tabs"), PrescriptionRequired: true,
	})
	if err != nil {
		t.Fatalf("expected synonyms to resolve, got %v", err)
	}
	if *r.medication.DosageForm != "tablet" || *r.medication.Route != "oral" || *r.medication.UnitOfMeasure != "tablet" {
		t.Fatalf("expected canonical codes, got %#v", r.medication)
	}
}

func TestMedicationRejectsUnconfiguredVocabularyValue(t *testing.T) {
	r := &catalogueRepo{}
	s := NewService(r, noopAudit{})
	_, err := s.CreateMedicationDefinition(context.Background(), "actor", MedicationCatalogueInput{
		Code: "MED-1", GenericName: "Amoxicillin", DosageForm: ptr("nonsense"), PrescriptionRequired: true,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected unknown dosage form to be rejected, got %v", err)
	}
	if !strings.Contains(err.Error(), "dosage form") {
		t.Fatalf("expected the message to name the field, got %q", err.Error())
	}
}

func TestMedicationVocabularyFieldsStayOptional(t *testing.T) {
	r := &catalogueRepo{}
	s := NewService(r, noopAudit{})
	_, err := s.CreateMedicationDefinition(context.Background(), "actor", MedicationCatalogueInput{
		Code: "MED-1", GenericName: "Amoxicillin", DosageForm: ptr("  "), PrescriptionRequired: true,
	})
	if err != nil {
		t.Fatalf("expected blank vocabulary fields to be allowed, got %v", err)
	}
	if r.medication.DosageForm != nil || r.medication.Route != nil {
		t.Fatalf("expected unset vocabulary fields, got %#v", r.medication)
	}
}

func TestMedicationImportNamesTheUnrecognisedColumn(t *testing.T) {
	r := &catalogueRepo{existing: map[string]bool{}}
	s := NewService(r, noopAudit{})
	csv := "code,generic_name,brand_name,strength,dosage_form,route,pack_size,unit_of_measure,prescription_required,reference_system,reference_code,active\n" +
		"MED-1,Amoxicillin,,500 mg,nonsense,Oral,21,Capsules,true,,,true\n"
	preview, err := s.PreviewCatalogueImport(context.Background(), "actor", "medications", "create_only", strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Issues) != 1 || preview.Issues[0].Code != "UNKNOWN_DOSAGE_FORM" {
		t.Fatalf("expected a dosage form issue, got %#v", preview.Issues)
	}
}

func TestImportPreviewIsAtomicAndRejectsExistingCodeInCreateMode(t *testing.T) {
	r := &catalogueRepo{existing: map[string]bool{"svc-1": true}}
	s := NewService(r, noopAudit{})
	csv := "code,name,category,department_code,service_point_code,price,currency,requires_order,requires_result,estimated_duration_minutes,reference_system,reference_code,active\nSVC-1,Consultation,consultation,,,500,KES,true,false,15,,,true\n"
	preview, err := s.PreviewCatalogueImport(context.Background(), "actor", "services", "create_only", strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if preview.Summary.Errors != 1 || r.saved != nil {
		t.Fatalf("expected invalid preview without staging, got %#v", preview)
	}
}

func TestValidMedicationImportStagesPreview(t *testing.T) {
	r := &catalogueRepo{existing: map[string]bool{}}
	s := NewService(r, noopAudit{})
	csv := "code,generic_name,brand_name,strength,dosage_form,route,pack_size,unit_of_measure,prescription_required,reference_system,reference_code,active\nMED-1,Amoxicillin,,500 mg,Capsule,Oral,21,Capsules,true,KEML,KEML-1,true\n"
	preview, err := s.PreviewCatalogueImport(context.Background(), "actor", "medications", "create_only", strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if preview.ImportID == "" || preview.Summary.Creates != 1 || r.saved == nil {
		t.Fatalf("expected staged preview, got %#v", preview)
	}
}
