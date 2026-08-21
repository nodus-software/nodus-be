package audit

import "time"

type Result string

const (
	ResultSuccess Result = "success"
	ResultFailure Result = "failure"
)

// Entry is one append-only audit trail record. Every security-sensitive
// action in the system (login, logout, password change, MFA changes, role
// changes, etc.) is recorded as an Entry. There is no update or delete path
// for these records anywhere in the codebase.
type Entry struct {
	ID                  string
	TenantID            string
	Timestamp           time.Time
	UserID              *string // nil when the actor is unknown/unauthenticated (e.g. a reset request for a non-existent username)
	TargetUserID        *string
	Action              string
	TargetResource      string
	IPAddress           string
	RequestID           string
	UserAgent           string
	ReasonCode          string
	PrivilegedReference string
	Result              Result
	Metadata            map[string]any
}
