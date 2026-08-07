package clinical

import (
	"context"
	"fmt"
	"strings"
)

// Starter reference data for the prescribing vocabularies configured under
// Clinical configuration -> Reference data. Facilities own these lists once
// seeded: entries can be renamed, added to or deactivated from the UI, so this
// only has to be a sensible default rather than an exhaustive standard.
//
// migrations/000016_prescribing_reference_data.up.sql seeds the same values for
// tenants that already existed; keep the two in step.

type VocabularyEntry struct{ Code, Name string }

var DefaultDosageForms = []VocabularyEntry{
	{"tablet", "Tablet"},
	{"capsule", "Capsule"},
	{"oral_solution", "Oral solution"},
	{"oral_suspension", "Oral suspension"},
	{"syrup", "Syrup"},
	{"powder_for_suspension", "Powder for suspension"},
	{"injection", "Injection"},
	{"infusion", "Infusion"},
	{"cream", "Cream"},
	{"ointment", "Ointment"},
	{"gel", "Gel"},
	{"lotion", "Lotion"},
	{"eye_drops", "Eye drops"},
	{"ear_drops", "Ear drops"},
	{"nasal_spray", "Nasal spray"},
	{"inhaler", "Inhaler"},
	{"nebuliser_solution", "Nebuliser solution"},
	{"suppository", "Suppository"},
	{"pessary", "Pessary"},
	{"transdermal_patch", "Transdermal patch"},
	{"granules", "Granules"},
	{"implant", "Implant"},
}

var DefaultRoutes = []VocabularyEntry{
	{"oral", "Oral"},
	{"sublingual", "Sublingual"},
	{"buccal", "Buccal"},
	{"intravenous", "Intravenous (IV)"},
	{"intramuscular", "Intramuscular (IM)"},
	{"subcutaneous", "Subcutaneous (SC)"},
	{"intradermal", "Intradermal"},
	{"intrathecal", "Intrathecal"},
	{"topical", "Topical"},
	{"transdermal", "Transdermal"},
	{"rectal", "Rectal"},
	{"vaginal", "Vaginal"},
	{"ophthalmic", "Ophthalmic (eye)"},
	{"otic", "Otic (ear)"},
	{"nasal", "Nasal"},
	{"inhalation", "Inhalation"},
}

// Units of measure describe what `pack_size` counts, not the dose unit.
var DefaultUnitsOfMeasure = []VocabularyEntry{
	{"tablet", "Tablet(s)"},
	{"capsule", "Capsule(s)"},
	{"bottle", "Bottle"},
	{"vial", "Vial"},
	{"ampoule", "Ampoule"},
	{"sachet", "Sachet"},
	{"tube", "Tube"},
	{"blister", "Blister pack"},
	{"pre_filled_syringe", "Pre-filled syringe"},
	{"suppository", "Suppository"},
	{"patch", "Patch"},
	{"inhaler", "Inhaler"},
	{"ml", "Millilitre (mL)"},
	{"g", "Gram (g)"},
	{"piece", "Piece"},
}

var DefaultPrescriptionFrequencies = []VocabularyEntry{
	{"OD", "Once daily"},
	{"TD", "Twice daily"},
	{"TDS", "Three times daily"},
	{"QDS", "Four times daily"},
	{"Q4H", "Every 4 hours"},
	{"Q6H", "Every 6 hours"},
	{"Q8H", "Every 8 hours"},
	{"Q12H", "Every 12 hours"},
	{"NOCTE", "At night"},
	{"PRN", "As needed"},
	{"STAT", "Immediately"},
}

var DefaultSpecimenTypes = []VocabularyEntry{
	{"whole_blood", "Whole blood"},
	{"serum", "Serum"},
	{"plasma", "Plasma"},
	{"urine", "Urine"},
	{"stool", "Stool"},
	{"sputum", "Sputum"},
	{"swab", "Swab"},
	{"cerebrospinal_fluid", "Cerebrospinal fluid"},
	{"tissue", "Tissue"},
}

// Abbreviations clinicians and spreadsheets use for the seeded codes. Applied
// before the vocabulary lookup so a CSV saying "Tab" or "IV" imports cleanly.
// The same mappings run once, in SQL, in migration 000016.
var (
	dosageFormSynonyms = map[string]string{
		"tab": "tablet", "tabs": "tablet", "tablets": "tablet",
		"cap": "capsule", "caps": "capsule", "capsules": "capsule",
		"susp": "oral_suspension", "suspension": "oral_suspension",
		"soln": "oral_solution", "solution": "oral_solution",
		"inj": "injection", "neb": "nebuliser_solution",
		"patch": "transdermal_patch",
	}
	routeSynonyms = map[string]string{
		"po": "oral", "iv": "intravenous", "im": "intramuscular",
		"sc": "subcutaneous", "sq": "subcutaneous", "subcut": "subcutaneous",
		"pr": "rectal", "pv": "vaginal", "top": "topical",
		"inhaled": "inhalation", "neb": "inhalation",
	}
	unitOfMeasureSynonyms = map[string]string{
		"mls": "ml", "millilitre": "ml", "milliliter": "ml", "millilitres": "ml",
		"gram": "g", "grams": "g", "gm": "g",
		"amp": "ampoule", "amps": "ampoule",
		"pcs": "piece", "each": "piece", "unit": "piece", "units": "piece",
		"tabs": "tablet", "tablets": "tablet",
		"caps": "capsule", "capsules": "capsule",
		"syringe": "pre_filled_syringe",
	}
)

// The resource kinds under which each vocabulary is served, so callers name the
// list rather than the table behind it.
const (
	DosageFormKind            = "dosage-forms"
	RouteKind                 = "routes"
	UnitOfMeasureKind         = "units-of-measure"
	PrescriptionFrequencyKind = "prescription-frequencies"
	SpecimenTypeKind          = "specimen-types"
)

func synonymsFor(kind string) map[string]string {
	switch kind {
	case DosageFormKind:
		return dosageFormSynonyms
	case RouteKind:
		return routeSynonyms
	case UnitOfMeasureKind:
		return unitOfMeasureSynonyms
	default:
		return nil
	}
}

// vocabularyLookup resolves free-text input to the canonical code a tenant has
// configured. `codes` maps lower(code) -> stored code for that tenant's active
// rows. Empty input is always valid and resolves to nil.
func vocabularyLookup(v *string, kind string, codes map[string]string) (*string, bool) {
	if v == nil {
		return nil, true
	}
	key := strings.ToLower(strings.TrimSpace(*v))
	if key == "" {
		return nil, true
	}
	if mapped, ok := synonymsFor(kind)[key]; ok {
		key = mapped
	}
	code, ok := codes[key]
	if !ok {
		return nil, false
	}
	return &code, true
}

// ResolveVocabularyOrNil maps a value onto a code the facility has configured,
// or nil when nothing matches. For paths where an unrecognised value should be
// dropped rather than rejected — reference-library adoption copies verbatim
// upstream text, and losing a dosage form beats failing the whole batch.
func ResolveVocabularyOrNil(v *string, kind string, codes map[string]string) *string {
	resolved, ok := vocabularyLookup(v, kind, codes)
	if !ok {
		return nil
	}
	return resolved
}

// medicationVocabularies holds all three lists for the current tenant. Load it
// once per request — the CSV importer resolves thousands of rows against it.
type medicationVocabularies struct{ dosageForms, routes, units map[string]string }

func (s *Service) loadMedicationVocabularies(c context.Context) (medicationVocabularies, error) {
	var v medicationVocabularies
	var e error
	if v.dosageForms, e = s.repo.VocabularyCodes(c, DosageFormKind); e != nil {
		return v, e
	}
	if v.routes, e = s.repo.VocabularyCodes(c, RouteKind); e != nil {
		return v, e
	}
	v.units, e = s.repo.VocabularyCodes(c, UnitOfMeasureKind)
	return v, e
}

// resolve rewrites a medication's vocabulary fields to the codes this facility
// has configured. It reports the first field whose value no list recognises, so
// callers can name it rather than rejecting the whole record anonymously.
func (v medicationVocabularies) resolve(in *MedicationCatalogueInput) (field, value string, ok bool) {
	// A miss only happens for a non-empty value, so the dereferences are safe.
	form, hit := vocabularyLookup(in.DosageForm, DosageFormKind, v.dosageForms)
	if !hit {
		return "dosage_form", strings.TrimSpace(*in.DosageForm), false
	}
	route, hit := vocabularyLookup(in.Route, RouteKind, v.routes)
	if !hit {
		return "route", strings.TrimSpace(*in.Route), false
	}
	unit, hit := vocabularyLookup(in.UnitOfMeasure, UnitOfMeasureKind, v.units)
	if !hit {
		return "unit_of_measure", strings.TrimSpace(*in.UnitOfMeasure), false
	}
	in.DosageForm, in.Route, in.UnitOfMeasure = form, route, unit
	return "", "", true
}

// resolveMedicationVocabulary is the single-record path used by create and update.
func (s *Service) resolveMedicationVocabulary(c context.Context, in *MedicationCatalogueInput) error {
	vocab, e := s.loadMedicationVocabularies(c)
	if e != nil {
		return e
	}
	if field, value, ok := vocab.resolve(in); !ok {
		return fmt.Errorf("%w: %q is not a configured %s", ErrInvalidInput, value, strings.ReplaceAll(field, "_", " "))
	}
	return nil
}
