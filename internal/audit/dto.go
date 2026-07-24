package audit

import "time"

type AuditLogEntryResponse struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	Timestamp      time.Time      `json:"timestamp"`
	UserID         *string        `json:"user_id,omitempty"`
	Action         string         `json:"action"`
	TargetResource string         `json:"target_resource"`
	IPAddress      string         `json:"ip_address"`
	Result         string         `json:"result"`
	Metadata       map[string]any `json:"metadata"`
}

func toAuditLogEntryResponse(e Entry) AuditLogEntryResponse {
	metadata := e.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	return AuditLogEntryResponse{
		ID: e.ID, TenantID: e.TenantID, Timestamp: e.Timestamp, UserID: e.UserID, Action: e.Action,
		TargetResource: e.TargetResource, IPAddress: e.IPAddress, Result: string(e.Result),
		Metadata: metadata,
	}
}
