package clinical

import "time"

type DiagnosisFilters struct {
	Query, Chapter, Availability string
	Page, PageSize               int
}
type DiagnosisConceptPage struct {
	Items    []Concept `json:"items"`
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
type SetDiagnosisAvailabilityRequest struct {
	Enabled bool `json:"enabled"`
}

type Allergen struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Category  string    `json:"category"`
	Aliases   []string  `json:"aliases"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type CreateAllergenRequest struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Aliases  []string `json:"aliases"`
}
type UpdateAllergenRequest struct {
	Code     *string   `json:"code,omitempty"`
	Name     *string   `json:"name,omitempty"`
	Category *string   `json:"category,omitempty"`
	Aliases  *[]string `json:"aliases,omitempty"`
}
