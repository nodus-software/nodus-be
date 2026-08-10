package clinical

import "time"

type ClinicalOrder struct {
	ID               string                 `json:"id"`
	PatientID        string                 `json:"patient_id"`
	VisitID          string                 `json:"visit_id"`
	EncounterID      string                 `json:"encounter_id"`
	Kind             string                 `json:"kind"`
	Category         string                 `json:"category"`
	Priority         int16                  `json:"priority"`
	Status           string                 `json:"status"`
	ReviewRequired   bool                   `json:"review_required"`
	OrderedBy        string                 `json:"ordered_by"`
	OrderedAt        time.Time              `json:"ordered_at"`
	CompletedAt      *time.Time             `json:"completed_at,omitempty"`
	CancelledAt      *time.Time             `json:"cancelled_at,omitempty"`
	TransitionReason *string                `json:"transition_reason,omitempty"`
	Version          int                    `json:"version"`
	UpdatedAt        time.Time              `json:"updated_at"`
	Service          *ServiceOrderDetail    `json:"service,omitempty"`
	Medication       *MedicationOrderDetail `json:"medication,omitempty"`
}

type ServiceOrderDetail struct {
	ServiceID   string `json:"service_id"`
	ServiceCode string `json:"service_code"`
	ServiceName string `json:"service_name"`
}

type MedicationOrderDetail struct {
	MedicationID          string     `json:"medication_id"`
	MedicationCode        string     `json:"medication_code"`
	MedicationName        string     `json:"medication_name"`
	Dose                  float64    `json:"dose"`
	DoseUnit              string     `json:"dose_unit"`
	Route                 string     `json:"route"`
	Frequency             string     `json:"frequency"`
	DurationDays          *int       `json:"duration_days,omitempty"`
	Quantity              *float64   `json:"quantity,omitempty"`
	Instructions          *string    `json:"instructions,omitempty"`
	AllergyOverrideReason *string    `json:"allergy_override_reason,omitempty"`
	AllergyAcknowledgedAt *time.Time `json:"allergy_acknowledged_at,omitempty"`
}

type CreateOrderRequest struct {
	EncounterID           string   `json:"encounter_id"`
	Kind                  string   `json:"kind"`
	Priority              int16    `json:"priority"`
	ReviewRequired        bool     `json:"review_required"`
	ServiceID             string   `json:"service_id,omitempty"`
	MedicationID          string   `json:"medication_id,omitempty"`
	Dose                  *float64 `json:"dose,omitempty"`
	DoseUnit              string   `json:"dose_unit,omitempty"`
	Route                 string   `json:"route,omitempty"`
	Frequency             string   `json:"frequency,omitempty"`
	DurationDays          *int     `json:"duration_days,omitempty"`
	Quantity              *float64 `json:"quantity,omitempty"`
	Instructions          *string  `json:"instructions,omitempty"`
	AllergyOverrideReason *string  `json:"allergy_override_reason,omitempty"`
}

type OrderFilters struct {
	VisitID, Category, Status, Kind string
	Page, PerPage                   int
}
type OrderPage struct {
	Data                             []ClinicalOrder `json:"data"`
	Page, PerPage, Total, TotalPages int
}
type TransitionOrderRequest struct {
	Status          string `json:"status"`
	ExpectedVersion int    `json:"expected_version"`
	Reason          string `json:"reason,omitempty"`
}

type OutpatientVisitContext struct {
	Visit             Visit           `json:"visit"`
	CurrentQueueEntry *QueueEntry     `json:"current_queue_entry,omitempty"`
	Encounters        []Encounter     `json:"encounters"`
	Allergies         []Allergy       `json:"allergies"`
	Orders            []ClinicalOrder `json:"orders"`
	WorkflowStage     string          `json:"workflow_stage"`
}

type OutpatientVisitFilters struct {
	Date, Status, Stage, ServicePointID, ClinicianID, Query string
	Page, PerPage                                           int
}
type OutpatientVisitListItem struct {
	Visit                  Visit   `json:"visit"`
	PatientName            string  `json:"patient_name"`
	PatientMRN             string  `json:"patient_mrn"`
	WorkflowStage          string  `json:"workflow_stage"`
	QueueStatus            *string `json:"queue_status,omitempty"`
	QueueName              *string `json:"queue_name,omitempty"`
	TriageCompleted        bool    `json:"triage_completed"`
	AttendingClinicianName *string `json:"attending_clinician_name,omitempty"`
	WaitingMinutes         int     `json:"waiting_minutes"`
}
