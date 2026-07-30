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
	Code    string `json:"code"`
	Display string `json:"display"`
	Version string `json:"version"`
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
