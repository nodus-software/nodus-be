package clinical

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var templateKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var templateTypes = map[string]bool{"short_text": true, "long_text": true, "number": true, "boolean": true, "single_select": true, "multi_select": true, "date": true}

func ValidateTemplateDefinition(d TemplateDefinition) error {
	if d.SchemaVersion != 1 || len(d.Sections) == 0 || len(d.Sections) > 20 {
		return ErrInvalidInput
	}
	keys := map[string]bool{}
	sectionKeys := map[string]bool{}
	fieldCount := 0
	for _, s := range d.Sections {
		if !templateKey.MatchString(s.Key) || sectionKeys[s.Key] || strings.TrimSpace(s.Title) == "" || len(s.Fields) == 0 {
			return ErrInvalidInput
		}
		sectionKeys[s.Key] = true
		for _, f := range s.Fields {
			fieldCount++
			if !templateKey.MatchString(f.Key) || keys[f.Key] || strings.TrimSpace(f.Label) == "" || !templateTypes[f.Type] {
				return ErrInvalidInput
			}
			keys[f.Key] = true
			selectType := f.Type == "single_select" || f.Type == "multi_select"
			if selectType != (len(f.Options) > 0) {
				return ErrInvalidInput
			}
			seenOptions := map[string]bool{}
			for _, o := range f.Options {
				if strings.TrimSpace(o.Value) == "" || strings.TrimSpace(o.Label) == "" || seenOptions[o.Value] {
					return ErrInvalidInput
				}
				seenOptions[o.Value] = true
			}
			if f.Validation != nil {
				v := f.Validation
				if v.Min != nil && v.Max != nil && *v.Min > *v.Max {
					return ErrInvalidInput
				}
				if v.MinLength != nil && *v.MinLength < 0 || v.MaxLength != nil && *v.MaxLength < 0 || v.MinLength != nil && v.MaxLength != nil && *v.MinLength > *v.MaxLength {
					return ErrInvalidInput
				}
				if f.Type != "number" && (v.Min != nil || v.Max != nil) {
					return ErrInvalidInput
				}
				if f.Type != "short_text" && f.Type != "long_text" && (v.MinLength != nil || v.MaxLength != nil) {
					return ErrInvalidInput
				}
			}
			if f.Binding != nil && (f.Binding.Kind != "observation" || strings.TrimSpace(f.Binding.Code) == "" || (f.Type != "number" && f.Type != "short_text" && f.Type != "long_text")) {
				return ErrInvalidInput
			}
		}
	}
	if fieldCount > 100 {
		return ErrInvalidInput
	}
	return nil
}

func validateFormAnswers(d TemplateDefinition, answers map[string]json.RawMessage, final bool) []FieldValidationError {
	fields := map[string]TemplateField{}
	for _, s := range d.Sections {
		for _, f := range s.Fields {
			fields[f.Key] = f
		}
	}
	errs := []FieldValidationError{}
	for key := range answers {
		if _, ok := fields[key]; !ok {
			errs = append(errs, FieldValidationError{Field: key, Message: "unknown field"})
		}
	}
	for key, f := range fields {
		raw, exists := answers[key]
		if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			if final && f.Required {
				errs = append(errs, FieldValidationError{Field: key, Message: "is required"})
			}
			continue
		}
		bad := func(message string) { errs = append(errs, FieldValidationError{Field: key, Message: message}) }
		switch f.Type {
		case "short_text", "long_text", "date", "single_select":
			var v string
			if json.Unmarshal(raw, &v) != nil {
				bad("must be a string")
				continue
			}
			if final && f.Required && strings.TrimSpace(v) == "" {
				bad("is required")
				continue
			}
			if f.Type == "date" {
				if _, e := time.Parse("2006-01-02", v); e != nil {
					bad("must be an ISO date")
				}
			}
			if f.Type == "single_select" && !optionExists(f.Options, v) {
				bad("must be one of the configured options")
			}
			if f.Validation != nil {
				if f.Validation.MinLength != nil && len([]rune(v)) < *f.Validation.MinLength {
					bad("is too short")
				}
				if f.Validation.MaxLength != nil && len([]rune(v)) > *f.Validation.MaxLength {
					bad("is too long")
				}
			}
		case "number":
			var v float64
			if json.Unmarshal(raw, &v) != nil {
				bad("must be a number")
				continue
			}
			if f.Validation != nil {
				if f.Validation.Min != nil && v < *f.Validation.Min {
					bad(fmt.Sprintf("must be at least %v", *f.Validation.Min))
				}
				if f.Validation.Max != nil && v > *f.Validation.Max {
					bad(fmt.Sprintf("must be at most %v", *f.Validation.Max))
				}
			}
		case "boolean":
			var v bool
			if json.Unmarshal(raw, &v) != nil {
				bad("must be a boolean")
			}
		case "multi_select":
			var v []string
			if json.Unmarshal(raw, &v) != nil {
				bad("must be an array of strings")
				continue
			}
			if final && f.Required && len(v) == 0 {
				bad("is required")
				continue
			}
			for _, x := range v {
				if !optionExists(f.Options, x) {
					bad("contains an unconfigured option")
					break
				}
			}
		}
	}
	return errs
}

func optionExists(options []TemplateOption, value string) bool {
	for _, o := range options {
		if o.Value == value {
			return true
		}
	}
	return false
}
