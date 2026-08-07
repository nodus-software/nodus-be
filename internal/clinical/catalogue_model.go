package clinical

import (
	"encoding/json"
	"time"
)

type CatalogueFilters struct {
	Query        string
	Status       string
	Category     string
	DepartmentID string
	Prescription string
	Page         int
	PerPage      int
}

type CatalogueService struct {
	ID                       string    `json:"id"`
	ReferenceItemID          *string   `json:"reference_item_id,omitempty"`
	Code                     string    `json:"code"`
	Name                     string    `json:"name"`
	Category                 string    `json:"category"`
	DepartmentID             *string   `json:"department_id,omitempty"`
	ServicePointID           *string   `json:"service_point_id,omitempty"`
	Price                    *float64  `json:"price,omitempty"`
	Currency                 string    `json:"currency"`
	RequiresOrder            bool      `json:"requires_order"`
	RequiresResult           bool      `json:"requires_result"`
	EstimatedDurationMinutes *int      `json:"estimated_duration_minutes,omitempty"`
	Active                   bool      `json:"active"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type ServiceCatalogueInput struct {
	ReferenceItemID          *string  `json:"reference_item_id,omitempty"`
	Code                     string   `json:"code"`
	Name                     string   `json:"name"`
	Category                 string   `json:"category"`
	DepartmentID             *string  `json:"department_id,omitempty"`
	ServicePointID           *string  `json:"service_point_id,omitempty"`
	Price                    *float64 `json:"price,omitempty"`
	Currency                 string   `json:"currency,omitempty"`
	RequiresOrder            bool     `json:"requires_order"`
	RequiresResult           bool     `json:"requires_result"`
	EstimatedDurationMinutes *int     `json:"estimated_duration_minutes,omitempty"`
	Active                   *bool    `json:"active,omitempty"`
}

type MedicationDefinition struct {
	ID                   string    `json:"id"`
	ReferenceItemID      *string   `json:"reference_item_id,omitempty"`
	Code                 string    `json:"code"`
	GenericName          string    `json:"generic_name"`
	BrandName            *string   `json:"brand_name,omitempty"`
	Strength             *string   `json:"strength,omitempty"`
	DosageForm           *string   `json:"dosage_form,omitempty"`
	Route                *string   `json:"route,omitempty"`
	PackSize             *string   `json:"pack_size,omitempty"`
	UnitOfMeasure        *string   `json:"unit_of_measure,omitempty"`
	PrescriptionRequired bool      `json:"prescription_required"`
	Active               bool      `json:"active"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type MedicationCatalogueInput struct {
	ReferenceItemID      *string `json:"reference_item_id,omitempty"`
	Code                 string  `json:"code"`
	GenericName          string  `json:"generic_name"`
	BrandName            *string `json:"brand_name,omitempty"`
	Strength             *string `json:"strength,omitempty"`
	DosageForm           *string `json:"dosage_form,omitempty"`
	Route                *string `json:"route,omitempty"`
	PackSize             *string `json:"pack_size,omitempty"`
	UnitOfMeasure        *string `json:"unit_of_measure,omitempty"`
	PrescriptionRequired bool    `json:"prescription_required"`
	Active               *bool   `json:"active,omitempty"`
}

type CatalogueReferenceItem struct {
	ID             string  `json:"id"`
	SourceSystem   string  `json:"source_system"`
	SourceVersion  string  `json:"source_version"`
	SourceCode     string  `json:"source_code"`
	Name           string  `json:"name"`
	Category       *string `json:"category,omitempty"`
	Strength       *string `json:"strength,omitempty"`
	DosageForm     *string `json:"dosage_form,omitempty"`
	Route          *string `json:"route,omitempty"`
	StandardSystem *string `json:"standard_system,omitempty"`
	StandardCode   *string `json:"standard_code,omitempty"`
}

type ServicePointImportReference struct {
	ID           string
	DepartmentID string
}

type ServiceAdoption struct {
	ReferenceItemID          string   `json:"reference_item_id"`
	Code                     string   `json:"code"`
	DepartmentID             *string  `json:"department_id,omitempty"`
	ServicePointID           *string  `json:"service_point_id,omitempty"`
	Price                    *float64 `json:"price,omitempty"`
	Currency                 string   `json:"currency,omitempty"`
	RequiresOrder            bool     `json:"requires_order"`
	RequiresResult           bool     `json:"requires_result"`
	EstimatedDurationMinutes *int     `json:"estimated_duration_minutes,omitempty"`
}

type MedicationAdoption struct {
	ReferenceItemID      string  `json:"reference_item_id"`
	Code                 string  `json:"code"`
	BrandName            *string `json:"brand_name,omitempty"`
	PackSize             *string `json:"pack_size,omitempty"`
	UnitOfMeasure        *string `json:"unit_of_measure,omitempty"`
	PrescriptionRequired bool    `json:"prescription_required"`
}

type ImportIssue struct {
	Row     int               `json:"row"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

type ImportSummary struct {
	Total     int `json:"total"`
	Creates   int `json:"creates"`
	Updates   int `json:"updates"`
	Unchanged int `json:"unchanged"`
	Errors    int `json:"errors"`
}

type ImportPreview struct {
	ImportID  string        `json:"import_id,omitempty"`
	Summary   ImportSummary `json:"summary"`
	Issues    []ImportIssue `json:"issues"`
	ExpiresAt *time.Time    `json:"expires_at,omitempty"`
}

type CatalogueImport struct {
	ID           string
	Catalogue    string
	Mode         string
	Status       string
	Rows         json.RawMessage
	Summary      json.RawMessage
	ExpiresAt    time.Time
	FileChecksum string
}
