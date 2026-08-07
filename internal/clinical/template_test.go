package clinical

import (
	"encoding/json"
	"errors"
	"testing"
)

func raw(v string) json.RawMessage { return json.RawMessage(v) }

func TestDefaultTemplateDefinitionsAreValid(t *testing.T) {
	for name, d := range map[string]TemplateDefinition{"triage": DefaultTriageTemplate, "consultation": DefaultConsultationTemplate} {
		if err := ValidateTemplateDefinition(d); err != nil {
			t.Fatalf("%s default is invalid: %v", name, err)
		}
	}
}

func TestTemplateDefinitionRejectsDuplicateFieldKeys(t *testing.T) {
	d := TemplateDefinition{SchemaVersion: 1, Sections: []TemplateSection{
		{Key: "one", Title: "One", Fields: []TemplateField{{Key: "same", Label: "First", Type: "short_text"}}},
		{Key: "two", Title: "Two", Fields: []TemplateField{{Key: "same", Label: "Second", Type: "short_text"}}},
	}}
	if !errors.Is(ValidateTemplateDefinition(d), ErrInvalidInput) {
		t.Fatal("expected duplicate field key to be rejected")
	}
}

func TestFormDraftAllowsMissingRequiredButSubmissionDoesNot(t *testing.T) {
	d := TemplateDefinition{SchemaVersion: 1, Sections: []TemplateSection{{Key: "main", Title: "Main", Fields: []TemplateField{{Key: "history", Label: "History", Type: "long_text", Required: true}}}}}
	if errs := validateFormAnswers(d, map[string]json.RawMessage{}, false); len(errs) != 0 {
		t.Fatalf("draft should allow missing fields: %#v", errs)
	}
	if errs := validateFormAnswers(d, map[string]json.RawMessage{}, true); len(errs) != 1 || errs[0].Field != "history" {
		t.Fatalf("expected required error: %#v", errs)
	}
}

func TestFormAnswersValidateTypeRangeAndOptions(t *testing.T) {
	min, max := 0.0, 10.0
	d := TemplateDefinition{SchemaVersion: 1, Sections: []TemplateSection{{Key: "main", Title: "Main", Fields: []TemplateField{
		{Key: "score", Label: "Score", Type: "number", Validation: &TemplateValidation{Min: &min, Max: &max}},
		{Key: "choice", Label: "Choice", Type: "single_select", Options: []TemplateOption{{Value: "yes", Label: "Yes"}}},
	}}}}
	errs := validateFormAnswers(d, map[string]json.RawMessage{"score": raw(`12`), "choice": raw(`"no"`), "extra": raw(`true`)}, true)
	if len(errs) != 3 {
		t.Fatalf("expected range, option and unknown errors, got %#v", errs)
	}
}
