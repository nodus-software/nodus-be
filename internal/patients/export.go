package patients

import (
	"encoding/csv"
	"net/http"

	"nodus-health/internal/audit"
)

// exportRowCap keeps GET /patients/export synchronous-safe: no background
// job queue exists in this codebase yet, so the whole file is generated
// and streamed within a single request.
const exportRowCap = 20000

var csvHeader = []string{"mrn", "full_name", "dob", "gender", "phone", "status", "national_id", "insured", "registered_at"}

func writeCSV(w http.ResponseWriter, list []PatientResponse) {
	cw := csv.NewWriter(w)
	_ = cw.Write(csvHeader)
	for _, p := range list {
		dob := ""
		if p.DOB != nil {
			dob = *p.DOB
		}
		phone := ""
		if p.Phone != nil {
			phone = *p.Phone
		}
		nationalID := ""
		if p.NationalID != nil {
			nationalID = *p.NationalID
		}
		insured := "false"
		if p.Insured {
			insured = "true"
		}
		_ = cw.Write([]string{
			p.MRN, p.FullName, dob, p.Gender, phone, p.Status, nationalID, insured,
			p.CreatedAt.Format(dateLayout),
		})
	}
	cw.Flush()
}

func auditExportEntry(actorUserID *string, rowCount int) audit.Entry {
	return audit.Entry{
		UserID: actorUserID, Action: "patient_list_exported", Result: audit.ResultSuccess,
		Metadata: map[string]any{"row_count": rowCount, "format": "csv"},
	}
}
