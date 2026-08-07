package clinical

import (
	"encoding/json"
	"time"
)

type TemplateDefinition struct {
	SchemaVersion int               `json:"schema_version"`
	Sections      []TemplateSection `json:"sections"`
}

type TemplateSection struct {
	Key         string          `json:"key"`
	Title       string          `json:"title"`
	Description *string         `json:"description,omitempty"`
	Fields      []TemplateField `json:"fields"`
}

type TemplateField struct {
	Key        string                `json:"key"`
	Label      string                `json:"label"`
	Type       string                `json:"type"`
	HelpText   *string               `json:"help_text,omitempty"`
	Required   bool                  `json:"required"`
	Options    []TemplateOption      `json:"options,omitempty"`
	Validation *TemplateValidation   `json:"validation,omitempty"`
	Binding    *TemplateFieldBinding `json:"binding,omitempty"`
}

type TemplateOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
type TemplateValidation struct {
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	MinLength *int     `json:"min_length,omitempty"`
	MaxLength *int     `json:"max_length,omitempty"`
}
type TemplateFieldBinding struct {
	Kind string  `json:"kind"`
	Code string  `json:"code"`
	Unit *string `json:"unit,omitempty"`
}

type ClinicalTemplate struct {
	ID            string                   `json:"id"`
	Code          string                   `json:"code"`
	Name          string                   `json:"name"`
	Description   *string                  `json:"description,omitempty"`
	EncounterType string                   `json:"encounter_type"`
	IsDefault     bool                     `json:"is_default"`
	ArchivedAt    *time.Time               `json:"archived_at,omitempty"`
	CreatedAt     time.Time                `json:"created_at"`
	UpdatedAt     time.Time                `json:"updated_at"`
	Published     *ClinicalTemplateVersion `json:"published,omitempty"`
	Draft         *ClinicalTemplateVersion `json:"draft,omitempty"`
}

type ClinicalTemplateVersion struct {
	ID          string             `json:"id"`
	TemplateID  string             `json:"template_id"`
	Version     int                `json:"version"`
	Status      string             `json:"status"`
	Definition  TemplateDefinition `json:"definition"`
	CreatedAt   time.Time          `json:"created_at"`
	PublishedAt *time.Time         `json:"published_at,omitempty"`
	CreatedBy   *string            `json:"-"`
}

type CreateTemplateRequest struct {
	Code          string             `json:"code"`
	Name          string             `json:"name"`
	Description   *string            `json:"description,omitempty"`
	EncounterType string             `json:"encounter_type"`
	Definition    TemplateDefinition `json:"definition"`
}
type UpdateTemplateDraftRequest struct {
	Name        *string            `json:"name,omitempty"`
	Description *string            `json:"description,omitempty"`
	Definition  TemplateDefinition `json:"definition"`
}
type ArchiveTemplateRequest struct {
	Reason string `json:"reason"`
}

type EncounterForm struct {
	ID              string                     `json:"id"`
	EncounterID     string                     `json:"encounter_id"`
	Template        ClinicalTemplate           `json:"template"`
	TemplateVersion ClinicalTemplateVersion    `json:"template_version"`
	Status          string                     `json:"status"`
	Answers         map[string]json.RawMessage `json:"answers"`
	Revision        int                        `json:"revision"`
	SavedBy         *string                    `json:"saved_by,omitempty"`
	SubmittedBy     *string                    `json:"submitted_by,omitempty"`
	CreatedAt       time.Time                  `json:"created_at"`
	UpdatedAt       time.Time                  `json:"updated_at"`
	SubmittedAt     *time.Time                 `json:"submitted_at,omitempty"`
}

type SaveEncounterFormRequest struct {
	Revision int                        `json:"revision"`
	Answers  map[string]json.RawMessage `json:"answers"`
}
type SubmitEncounterFormRequest struct {
	Revision int `json:"revision"`
}

type FieldValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}
type FormValidationError struct {
	Errors []FieldValidationError `json:"errors"`
}

func (e *FormValidationError) Error() string { return "clinical form validation failed" }
