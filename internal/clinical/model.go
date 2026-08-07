package clinical

import "time"

type Resource struct {
	ID             string    `json:"id"`
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	Active         bool      `json:"active"`
	DepartmentID   *string   `json:"department_id,omitempty"`
	WardID         *string   `json:"ward_id,omitempty"`
	RoomID         *string   `json:"room_id,omitempty"`
	ServicePointID *string   `json:"service_point_id,omitempty"`
	Kind           *string   `json:"kind,omitempty"`
	Status         *string   `json:"status,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}
type Visit struct {
	ID        string     `json:"id"`
	PatientID string     `json:"patient_id"`
	VisitType string     `json:"visit_type"`
	Status    string     `json:"status"`
	Reason    *string    `json:"reason,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
}
type QueueEntry struct {
	ID               string    `json:"id"`
	QueueID          string    `json:"queue_id"`
	QueueName        string    `json:"queue_name"`
	SubjectType      string    `json:"subject_type"`
	SubjectID        string    `json:"subject_id"`
	PatientID        string    `json:"patient_id"`
	PatientName      string    `json:"patient_name"`
	PatientMRN       string    `json:"patient_mrn"`
	Status           string    `json:"status"`
	Priority         int16     `json:"priority"`
	Acuity           *int16    `json:"acuity,omitempty"`
	PositionOverride *int      `json:"position_override,omitempty"`
	JoinedAt         time.Time `json:"joined_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
type QueueHistory struct {
	ID          string    `json:"id"`
	FromStatus  *string   `json:"from_status,omitempty"`
	ToStatus    string    `json:"to_status"`
	FromQueueID *string   `json:"from_queue_id,omitempty"`
	ToQueueID   string    `json:"to_queue_id"`
	ActorID     *string   `json:"actor_id,omitempty"`
	Reason      *string   `json:"reason,omitempty"`
	Automated   bool      `json:"automated"`
	OccurredAt  time.Time `json:"occurred_at"`
}
type RoutingRule struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	EventType     string  `json:"event_type"`
	VisitType     *string `json:"visit_type,omitempty"`
	TargetQueueID string  `json:"target_queue_id"`
	Priority      int16   `json:"priority"`
	Active        bool    `json:"active"`
}
type Concept struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	Display string `json:"display"`
	Version string `json:"version"`
	URI     string `json:"uri,omitempty"`
	Chapter string `json:"chapter,omitempty"`
	Enabled bool   `json:"enabled"`
}
type CreateResourceRequest struct {
	Code           string  `json:"code"`
	Name           string  `json:"name"`
	DepartmentID   *string `json:"department_id,omitempty"`
	WardID         *string `json:"ward_id,omitempty"`
	RoomID         *string `json:"room_id,omitempty"`
	ServicePointID *string `json:"service_point_id,omitempty"`
	Kind           *string `json:"kind,omitempty"`
}

type UpdateResourceRequest struct {
	Code *string `json:"code,omitempty"`
	Name *string `json:"name,omitempty"`
	Kind *string `json:"kind,omitempty"`
}

type DeactivateResourceRequest struct {
	Reason  string `json:"reason"`
	Cascade bool   `json:"cascade"`
}

type ResourceReference struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type OperationalBlocker struct {
	Type       string `json:"type"`
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Name       string `json:"name"`
	Count      int    `json:"count"`
	Message    string `json:"message"`
}

type DeactivationImpact struct {
	Root                ResourceReference    `json:"root"`
	ActiveDescendants   []ResourceReference  `json:"active_descendants"`
	DescendantCounts    map[string]int       `json:"descendant_counts"`
	OperationalBlockers []OperationalBlocker `json:"operational_blockers"`
	CascadeAllowed      bool                 `json:"cascade_allowed"`
}

type ResourceLifecycleResult struct {
	Root     ResourceReference   `json:"root"`
	Affected []ResourceReference `json:"affected"`
}
type CreateVisitRequest struct {
	PatientID string  `json:"patient_id"`
	VisitType string  `json:"visit_type"`
	Reason    *string `json:"reason,omitempty"`
}
type EnqueueRequest struct {
	SubjectType string `json:"subject_type"`
	SubjectID   string `json:"subject_id"`
	PatientID   string `json:"patient_id"`
	Priority    int16  `json:"priority"`
	Acuity      *int16 `json:"acuity,omitempty"`
	Reason      string `json:"reason"`
}
type TransitionRequest struct {
	Status   string  `json:"status"`
	QueueID  *string `json:"queue_id,omitempty"`
	Priority *int16  `json:"priority,omitempty"`
	Position *int    `json:"position,omitempty"`
	Reason   string  `json:"reason"`
}
type CreateRoutingRuleRequest struct {
	Name          string  `json:"name"`
	EventType     string  `json:"event_type"`
	VisitType     *string `json:"visit_type,omitempty"`
	TargetQueueID string  `json:"target_queue_id"`
	Priority      int16   `json:"priority"`
}

type Encounter struct {
	ID             string     `json:"id"`
	VisitID        string     `json:"visit_id"`
	EncounterType  string     `json:"encounter_type"`
	Status         string     `json:"status"`
	ServicePointID *string    `json:"service_point_id,omitempty"`
	ClinicianID    *string    `json:"clinician_id,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type ObservationInput struct {
	Code         string   `json:"code"`
	ValueNumeric *float64 `json:"value_numeric,omitempty"`
	ValueText    *string  `json:"value_text,omitempty"`
	Unit         *string  `json:"unit,omitempty"`
}
type Observation struct {
	ID           string    `json:"id"`
	PatientID    string    `json:"patient_id"`
	VisitID      string    `json:"visit_id"`
	Code         string    `json:"code"`
	RecordedBy   string    `json:"recorded_by"`
	EncounterID  *string   `json:"encounter_id,omitempty"`
	ValueNumeric *float64  `json:"value_numeric,omitempty"`
	ValueText    *string   `json:"value_text,omitempty"`
	Unit         *string   `json:"unit,omitempty"`
	ObservedAt   time.Time `json:"observed_at"`
	CreatedAt    time.Time `json:"created_at"`
}
type ClinicalNote struct {
	ID            string    `json:"id"`
	PatientID     string    `json:"patient_id"`
	VisitID       string    `json:"visit_id"`
	NoteType      string    `json:"note_type"`
	Body          string    `json:"body"`
	AuthoredBy    string    `json:"authored_by"`
	EncounterID   *string   `json:"encounter_id,omitempty"`
	AmendedFromID *string   `json:"amended_from_id,omitempty"`
	AuthoredAt    time.Time `json:"authored_at"`
}
type Diagnosis struct {
	ID          string    `json:"id"`
	PatientID   string    `json:"patient_id"`
	VisitID     string    `json:"visit_id"`
	ConceptID   string    `json:"concept_id"`
	Code        string    `json:"code"`
	Display     string    `json:"display"`
	Kind        string    `json:"kind"`
	RecordedBy  string    `json:"recorded_by"`
	EncounterID *string   `json:"encounter_id,omitempty"`
	Note        *string   `json:"note,omitempty"`
	RecordedAt  time.Time `json:"recorded_at"`
}
type Allergy struct {
	ID           string    `json:"id"`
	PatientID    string    `json:"patient_id"`
	Allergen     string    `json:"allergen"`
	Status       string    `json:"status"`
	RecordedBy   string    `json:"recorded_by"`
	Reaction     *string   `json:"reaction,omitempty"`
	Severity     *string   `json:"severity,omitempty"`
	RecordedAt   time.Time `json:"recorded_at"`
	AllergenID   *string   `json:"allergen_id,omitempty"`
	AllergenCode *string   `json:"allergen_code,omitempty"`
	Category     *string   `json:"category,omitempty"`
	IsCustom     bool      `json:"is_custom"`
}
type VisitSummary struct {
	Visit        Visit          `json:"visit"`
	Encounters   []Encounter    `json:"encounters"`
	Observations []Observation  `json:"observations"`
	Notes        []ClinicalNote `json:"notes"`
	Diagnoses    []Diagnosis    `json:"diagnoses"`
	Allergies    []Allergy      `json:"allergies"`
}
type OutpatientCheckInRequest struct {
	PatientID      string  `json:"patient_id"`
	Reason         *string `json:"reason,omitempty"`
	Override       bool    `json:"override,omitempty"`
	OverrideReason *string `json:"override_reason,omitempty"`
}
type CreateEncounterRequest struct {
	EncounterType  string  `json:"encounter_type"`
	ServicePointID *string `json:"service_point_id,omitempty"`
}
type RecordObservationsRequest struct {
	Observations []ObservationInput `json:"observations"`
}
type CreateNoteRequest struct {
	EncounterID *string `json:"encounter_id,omitempty"`
	NoteType    string  `json:"note_type"`
	Body        string  `json:"body"`
}
type CreateDiagnosisRequest struct {
	EncounterID *string `json:"encounter_id,omitempty"`
	Code        string  `json:"code"`
	ConceptID   string  `json:"concept_id,omitempty"`
	Kind        string  `json:"kind"`
	Note        *string `json:"note,omitempty"`
}
type CreateAllergyRequest struct {
	Allergen      string  `json:"allergen,omitempty"`
	AllergenID    string  `json:"allergen_id,omitempty"`
	OtherAllergen string  `json:"other_allergen,omitempty"`
	Reaction      *string `json:"reaction,omitempty"`
	Severity      *string `json:"severity,omitempty"`
}
type CompleteEncounterRequest struct {
	ConsultationQueueID *string `json:"consultation_queue_id,omitempty"`
}
