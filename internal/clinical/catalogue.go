package clinical

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"nodus-health/internal/audit"
	"nodus-health/pkg/utility"
)

var serviceCategories = map[string]bool{"laboratory": true, "imaging": true, "procedure": true, "nursing_service": true, "consultation": true, "therapy": true, "other": true}

func cleanOptional(v *string) *string {
	if v == nil {
		return nil
	}
	x := strings.TrimSpace(*v)
	if x == "" {
		return nil
	}
	return &x
}
func validCode(v string) bool { v = strings.TrimSpace(v); return v != "" && len(v) <= 100 }

func serviceFromInput(id string, in ServiceCatalogueInput) (CatalogueService, error) {
	in.Code = strings.TrimSpace(in.Code)
	in.Name = strings.TrimSpace(in.Name)
	in.Category = strings.TrimSpace(in.Category)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.Currency == "" {
		in.Currency = "KES"
	}
	if !validCode(in.Code) || in.Name == "" || !serviceCategories[in.Category] || len(in.Currency) != 3 {
		return CatalogueService{}, ErrInvalidInput
	}
	if in.Price != nil && (*in.Price < 0 || *in.Price > 999999999) {
		return CatalogueService{}, ErrInvalidInput
	}
	if in.EstimatedDurationMinutes != nil && *in.EstimatedDurationMinutes <= 0 {
		return CatalogueService{}, ErrInvalidInput
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	return CatalogueService{ID: id, ReferenceItemID: cleanOptional(in.ReferenceItemID), Code: in.Code, Name: in.Name, Category: in.Category, DepartmentID: cleanOptional(in.DepartmentID), ServicePointID: cleanOptional(in.ServicePointID), Price: in.Price, Currency: in.Currency, RequiresOrder: in.RequiresOrder, RequiresResult: in.RequiresResult, EstimatedDurationMinutes: in.EstimatedDurationMinutes, Active: active}, nil
}

func medicationFromInput(id string, in MedicationCatalogueInput) (MedicationDefinition, error) {
	in.Code = strings.TrimSpace(in.Code)
	in.GenericName = strings.TrimSpace(in.GenericName)
	if !validCode(in.Code) || in.GenericName == "" {
		return MedicationDefinition{}, ErrInvalidInput
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	return MedicationDefinition{ID: id, ReferenceItemID: cleanOptional(in.ReferenceItemID), Code: in.Code, GenericName: in.GenericName, BrandName: cleanOptional(in.BrandName), Strength: cleanOptional(in.Strength), DosageForm: cleanOptional(in.DosageForm), Route: cleanOptional(in.Route), PackSize: cleanOptional(in.PackSize), UnitOfMeasure: cleanOptional(in.UnitOfMeasure), PrescriptionRequired: in.PrescriptionRequired, Active: active}, nil
}

func normalizeFilters(f CatalogueFilters) CatalogueFilters {
	f.Query = strings.TrimSpace(f.Query)
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PerPage < 1 {
		f.PerPage = 25
	}
	if f.PerPage > 100 {
		f.PerPage = 100
	}
	return f
}
func (s *Service) ListCatalogueServices(c context.Context, f CatalogueFilters) ([]CatalogueService, int, error) {
	return s.repo.ListCatalogueServices(c, normalizeFilters(f))
}
func (s *Service) ListMedicationCatalogue(c context.Context, f CatalogueFilters) ([]MedicationDefinition, int, error) {
	return s.repo.ListMedicationCatalogue(c, normalizeFilters(f))
}
func (s *Service) ListCatalogueReferences(c context.Context, kind string, f CatalogueFilters) ([]CatalogueReferenceItem, int, error) {
	if kind != "services" && kind != "medications" {
		return nil, 0, ErrInvalidInput
	}
	return s.repo.ListCatalogueReferences(c, kind, normalizeFilters(f))
}

func (s *Service) CreateCatalogueService(c context.Context, actor string, in ServiceCatalogueInput) (*CatalogueService, error) {
	id, e := utility.GenerateUUID()
	if e != nil {
		return nil, e
	}
	x, e := serviceFromInput(id, in)
	if e != nil {
		return nil, e
	}
	out, e := s.repo.CreateCatalogueService(c, x)
	if e == nil {
		s.recordCatalogueAudit(c, actor, "service_catalogue_created", out.ID, map[string]any{"code": out.Code})
	}
	return out, e
}
func (s *Service) UpdateCatalogueService(c context.Context, actor, id string, in ServiceCatalogueInput) (*CatalogueService, error) {
	if id == "" {
		return nil, ErrInvalidInput
	}
	x, e := serviceFromInput(id, in)
	if e != nil {
		return nil, e
	}
	out, e := s.repo.UpdateCatalogueService(c, id, x)
	if e == nil {
		s.recordCatalogueAudit(c, actor, "service_catalogue_updated", id, nil)
	}
	return out, e
}
func (s *Service) SetCatalogueServiceActive(c context.Context, actor, id string, active bool, reason string) (*CatalogueService, error) {
	if id == "" || (!active && strings.TrimSpace(reason) == "") {
		return nil, ErrReasonRequired
	}
	out, e := s.repo.SetCatalogueServiceActive(c, id, active)
	if e == nil {
		s.recordCatalogueAudit(c, actor, map[bool]string{true: "service_catalogue_reactivated", false: "service_catalogue_deactivated"}[active], id, map[string]any{"reason": strings.TrimSpace(reason)})
	}
	return out, e
}
func (s *Service) CreateMedicationDefinition(c context.Context, actor string, in MedicationCatalogueInput) (*MedicationDefinition, error) {
	id, e := utility.GenerateUUID()
	if e != nil {
		return nil, e
	}
	if e = s.resolveMedicationVocabulary(c, &in); e != nil {
		return nil, e
	}
	x, e := medicationFromInput(id, in)
	if e != nil {
		return nil, e
	}
	out, e := s.repo.CreateMedicationDefinition(c, x)
	if e == nil {
		s.recordCatalogueAudit(c, actor, "medication_catalogue_created", out.ID, map[string]any{"code": out.Code})
	}
	return out, e
}
func (s *Service) UpdateMedicationDefinition(c context.Context, actor, id string, in MedicationCatalogueInput) (*MedicationDefinition, error) {
	if id == "" {
		return nil, ErrInvalidInput
	}
	if e := s.resolveMedicationVocabulary(c, &in); e != nil {
		return nil, e
	}
	x, e := medicationFromInput(id, in)
	if e != nil {
		return nil, e
	}
	out, e := s.repo.UpdateMedicationDefinition(c, id, x)
	if e == nil {
		s.recordCatalogueAudit(c, actor, "medication_catalogue_updated", id, nil)
	}
	return out, e
}
func (s *Service) SetMedicationDefinitionActive(c context.Context, actor, id string, active bool, reason string) (*MedicationDefinition, error) {
	if id == "" || (!active && strings.TrimSpace(reason) == "") {
		return nil, ErrReasonRequired
	}
	out, e := s.repo.SetMedicationDefinitionActive(c, id, active)
	if e == nil {
		s.recordCatalogueAudit(c, actor, map[bool]string{true: "medication_catalogue_reactivated", false: "medication_catalogue_deactivated"}[active], id, map[string]any{"reason": strings.TrimSpace(reason)})
	}
	return out, e
}
func (s *Service) recordCatalogueAudit(c context.Context, actor, action, target string, metadata map[string]any) {
	if s.audit != nil {
		_ = s.audit.Record(c, audit.Entry{UserID: &actor, Action: action, Result: audit.ResultSuccess, TargetResource: target, Metadata: metadata})
	}
}

func (s *Service) AdoptServiceReferences(c context.Context, actor string, in []ServiceAdoption) ([]CatalogueService, error) {
	if len(in) == 0 || len(in) > 500 {
		return nil, ErrInvalidInput
	}
	items := make([]CatalogueService, 0, len(in))
	seen := map[string]bool{}
	for _, v := range in {
		code := strings.TrimSpace(v.Code)
		key := strings.ToLower(code)
		if v.ReferenceItemID == "" || !validCode(code) || seen[key] {
			return nil, ErrInvalidInput
		}
		seen[key] = true
		id, e := utility.GenerateUUID()
		if e != nil {
			return nil, e
		}
		currency := strings.ToUpper(strings.TrimSpace(v.Currency))
		if currency == "" {
			currency = "KES"
		}
		active := true
		items = append(items, CatalogueService{ID: id, ReferenceItemID: &v.ReferenceItemID, Code: code, DepartmentID: cleanOptional(v.DepartmentID), ServicePointID: cleanOptional(v.ServicePointID), Price: v.Price, Currency: currency, RequiresOrder: v.RequiresOrder, RequiresResult: v.RequiresResult, EstimatedDurationMinutes: v.EstimatedDurationMinutes, Active: active})
	}
	out, e := s.repo.AdoptServiceReferences(c, items)
	if e == nil {
		s.recordCatalogueAudit(c, actor, "service_catalogue_references_adopted", "batch", map[string]any{"count": len(out)})
	}
	return out, e
}
func (s *Service) AdoptMedicationReferences(c context.Context, actor string, in []MedicationAdoption) ([]MedicationDefinition, error) {
	if len(in) == 0 || len(in) > 500 {
		return nil, ErrInvalidInput
	}
	items := make([]MedicationDefinition, 0, len(in))
	seen := map[string]bool{}
	for _, v := range in {
		code := strings.TrimSpace(v.Code)
		key := strings.ToLower(code)
		if v.ReferenceItemID == "" || !validCode(code) || seen[key] {
			return nil, ErrInvalidInput
		}
		seen[key] = true
		id, e := utility.GenerateUUID()
		if e != nil {
			return nil, e
		}
		items = append(items, MedicationDefinition{ID: id, ReferenceItemID: &v.ReferenceItemID, Code: code, BrandName: cleanOptional(v.BrandName), PackSize: cleanOptional(v.PackSize), UnitOfMeasure: cleanOptional(v.UnitOfMeasure), PrescriptionRequired: v.PrescriptionRequired, Active: true})
	}
	out, e := s.repo.AdoptMedicationReferences(c, items)
	if e == nil {
		s.recordCatalogueAudit(c, actor, "medication_catalogue_references_adopted", "batch", map[string]any{"count": len(out)})
	}
	return out, e
}

var serviceImportHeaders = []string{"code", "name", "category", "department_code", "service_point_code", "price", "currency", "requires_order", "requires_result", "estimated_duration_minutes", "reference_system", "reference_code", "active"}
var medicationImportHeaders = []string{"code", "generic_name", "brand_name", "strength", "dosage_form", "route", "pack_size", "unit_of_measure", "prescription_required", "reference_system", "reference_code", "active"}

func CatalogueTemplate(kind string) (string, error) {
	headers := serviceImportHeaders
	if kind == "medications" {
		headers = medicationImportHeaders
	} else if kind != "services" {
		return "", ErrInvalidInput
	}
	return strings.Join(headers, ",") + "\n", nil
}

func parseBoolCell(v string) (bool, error) {
	return strconv.ParseBool(strings.ToLower(strings.TrimSpace(v)))
}
func ptr(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}
func rowMap(headers, row []string) map[string]string {
	out := map[string]string{}
	for i, h := range headers {
		if i < len(row) {
			out[h] = strings.TrimSpace(row[i])
		}
	}
	return out
}
func sameHeaders(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		got[i] = strings.TrimPrefix(strings.TrimSpace(got[i]), "\ufeff")
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func (s *Service) PreviewCatalogueImport(c context.Context, actor, kind, mode string, r io.Reader) (*ImportPreview, error) {
	if kind != "services" && kind != "medications" || mode != "create_only" && mode != "upsert" {
		return nil, ErrInvalidInput
	}
	cr := csv.NewReader(io.LimitReader(r, 5<<20+1))
	cr.ReuseRecord = false
	headers, e := cr.Read()
	if e != nil {
		return nil, ErrInvalidInput
	}
	want := serviceImportHeaders
	if kind == "medications" {
		want = medicationImportHeaders
	}
	if !sameHeaders(headers, want) {
		return &ImportPreview{Summary: ImportSummary{Errors: 1}, Issues: []ImportIssue{{Row: 1, Code: "INVALID_HEADERS", Message: "CSV headers do not match the template"}}}, nil
	}
	// Loaded once: the loop below resolves up to 10000 rows against these.
	var vocab medicationVocabularies
	if kind == "medications" {
		if vocab, e = s.loadMedicationVocabularies(c); e != nil {
			return nil, e
		}
	}
	services := []CatalogueService{}
	meds := []MedicationDefinition{}
	type importLinks struct {
		index, row                                               int
		department, servicePoint, referenceSystem, referenceCode string
	}
	serviceLinks := []importLinks{}
	medicationLinks := []importLinks{}
	departmentCodes, servicePointCodes, referenceKeys := []string{}, []string{}, []string{}
	issues := []ImportIssue{}
	seen := map[string]int{}
	codes := []string{}
	rowNumber := 1
	for {
		row, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		rowNumber++
		if err != nil {
			issues = append(issues, ImportIssue{Row: rowNumber, Code: "INVALID_CSV", Message: err.Error()})
			continue
		}
		if rowNumber > 10001 {
			return nil, fmt.Errorf("%w: import exceeds 10000 rows", ErrInvalidInput)
		}
		m := rowMap(headers, row)
		key := strings.ToLower(m["code"])
		if key == "" {
			issues = append(issues, ImportIssue{Row: rowNumber, Code: "REQUIRED", Message: "code is required"})
			continue
		}
		if first := seen[key]; first > 0 {
			issues = append(issues, ImportIssue{Row: rowNumber, Code: "DUPLICATE_FILE_CODE", Message: fmt.Sprintf("code duplicates row %d", first)})
			continue
		}
		seen[key] = rowNumber
		codes = append(codes, key)
		id, err := utility.GenerateUUID()
		if err != nil {
			return nil, err
		}
		active, be := parseBoolCell(m["active"])
		if be != nil {
			issues = append(issues, ImportIssue{Row: rowNumber, Code: "INVALID_BOOLEAN", Message: "active must be true or false"})
			continue
		}
		if kind == "services" {
			requiresOrder, e1 := parseBoolCell(m["requires_order"])
			requiresResult, e2 := parseBoolCell(m["requires_result"])
			var price *float64
			if m["price"] != "" {
				v, e := strconv.ParseFloat(m["price"], 64)
				if e != nil || v < 0 {
					issues = append(issues, ImportIssue{Row: rowNumber, Code: "INVALID_PRICE", Message: "price must be a non-negative number"})
					continue
				}
				price = &v
			}
			var duration *int
			if m["estimated_duration_minutes"] != "" {
				v, e := strconv.Atoi(m["estimated_duration_minutes"])
				if e != nil || v <= 0 {
					issues = append(issues, ImportIssue{Row: rowNumber, Code: "INVALID_DURATION", Message: "duration must be a positive whole number"})
					continue
				}
				duration = &v
			}
			if e1 != nil || e2 != nil {
				issues = append(issues, ImportIssue{Row: rowNumber, Code: "INVALID_BOOLEAN", Message: "requires_order and requires_result must be true or false"})
				continue
			}
			a := active
			v, e := serviceFromInput(id, ServiceCatalogueInput{Code: m["code"], Name: m["name"], Category: m["category"], Price: price, Currency: m["currency"], RequiresOrder: requiresOrder, RequiresResult: requiresResult, EstimatedDurationMinutes: duration, Active: &a})
			if e != nil {
				issues = append(issues, ImportIssue{Row: rowNumber, Code: "INVALID_ROW", Message: "service fields are invalid"})
				continue
			}
			services = append(services, v)
			link := importLinks{index: len(services) - 1, row: rowNumber, department: strings.ToLower(m["department_code"]), servicePoint: strings.ToLower(m["service_point_code"]), referenceSystem: strings.ToLower(m["reference_system"]), referenceCode: strings.ToLower(m["reference_code"])}
			serviceLinks = append(serviceLinks, link)
			if link.department != "" {
				departmentCodes = append(departmentCodes, link.department)
			}
			if link.servicePoint != "" {
				servicePointCodes = append(servicePointCodes, link.servicePoint)
			}
			if link.referenceSystem != "" && link.referenceCode != "" {
				referenceKeys = append(referenceKeys, link.referenceSystem+"|"+link.referenceCode)
			}
		} else {
			prescription, e := parseBoolCell(m["prescription_required"])
			if e != nil {
				issues = append(issues, ImportIssue{Row: rowNumber, Code: "INVALID_BOOLEAN", Message: "prescription_required must be true or false"})
				continue
			}
			a := active
			in := MedicationCatalogueInput{Code: m["code"], GenericName: m["generic_name"], BrandName: ptr(m["brand_name"]), Strength: ptr(m["strength"]), DosageForm: ptr(m["dosage_form"]), Route: ptr(m["route"]), PackSize: ptr(m["pack_size"]), UnitOfMeasure: ptr(m["unit_of_measure"]), PrescriptionRequired: prescription, Active: &a}
			// Names the offending column rather than reporting a blanket invalid row.
			if field, value, ok := vocab.resolve(&in); !ok {
				issues = append(issues, ImportIssue{Row: rowNumber, Code: "UNKNOWN_" + strings.ToUpper(field), Message: fmt.Sprintf("%q is not a configured %s", value, strings.ReplaceAll(field, "_", " ")), Fields: map[string]string{field: value}})
				continue
			}
			v, e := medicationFromInput(id, in)
			if e != nil {
				issues = append(issues, ImportIssue{Row: rowNumber, Code: "INVALID_ROW", Message: "medication fields are invalid"})
				continue
			}
			meds = append(meds, v)
			link := importLinks{index: len(meds) - 1, row: rowNumber, referenceSystem: strings.ToLower(m["reference_system"]), referenceCode: strings.ToLower(m["reference_code"])}
			medicationLinks = append(medicationLinks, link)
			if link.referenceSystem != "" && link.referenceCode != "" {
				referenceKeys = append(referenceKeys, link.referenceSystem+"|"+link.referenceCode)
			}
		}
	}
	if kind == "services" {
		departments, e := s.repo.ResolveDepartmentCodes(c, departmentCodes)
		if e != nil {
			return nil, e
		}
		servicePoints, e := s.repo.ResolveServicePointCodes(c, servicePointCodes)
		if e != nil {
			return nil, e
		}
		references, e := s.repo.ResolveCatalogueReferenceCodes(c, kind, referenceKeys)
		if e != nil {
			return nil, e
		}
		for _, link := range serviceLinks {
			x := &services[link.index]
			if link.department != "" {
				id, ok := departments[link.department]
				if !ok {
					issues = append(issues, ImportIssue{Row: link.row, Code: "UNKNOWN_DEPARTMENT", Message: "department_code was not found or is inactive"})
				} else {
					x.DepartmentID = &id
				}
			}
			if link.servicePoint != "" {
				ref, ok := servicePoints[link.servicePoint]
				if !ok {
					issues = append(issues, ImportIssue{Row: link.row, Code: "UNKNOWN_SERVICE_POINT", Message: "service_point_code was not found or is inactive"})
				} else {
					x.ServicePointID = &ref.ID
					if x.DepartmentID != nil && *x.DepartmentID != ref.DepartmentID {
						issues = append(issues, ImportIssue{Row: link.row, Code: "LOCATION_MISMATCH", Message: "service point does not belong to the supplied department"})
					}
				}
			}
			if (link.referenceSystem == "") != (link.referenceCode == "") {
				issues = append(issues, ImportIssue{Row: link.row, Code: "INCOMPLETE_REFERENCE", Message: "reference_system and reference_code must be supplied together"})
			} else if link.referenceSystem != "" {
				id, ok := references[link.referenceSystem+"|"+link.referenceCode]
				if !ok {
					issues = append(issues, ImportIssue{Row: link.row, Code: "UNKNOWN_REFERENCE", Message: "reference code was not found in an active approved release"})
				} else {
					x.ReferenceItemID = &id
				}
			}
		}
	} else {
		references, e := s.repo.ResolveCatalogueReferenceCodes(c, kind, referenceKeys)
		if e != nil {
			return nil, e
		}
		for _, link := range medicationLinks {
			x := &meds[link.index]
			if (link.referenceSystem == "") != (link.referenceCode == "") {
				issues = append(issues, ImportIssue{Row: link.row, Code: "INCOMPLETE_REFERENCE", Message: "reference_system and reference_code must be supplied together"})
			} else if link.referenceSystem != "" {
				id, ok := references[link.referenceSystem+"|"+link.referenceCode]
				if !ok {
					issues = append(issues, ImportIssue{Row: link.row, Code: "UNKNOWN_REFERENCE", Message: "reference code was not found in an active approved release"})
				} else {
					x.ReferenceItemID = &id
				}
			}
		}
	}
	existing, e := s.repo.CatalogueCodes(c, kind, codes)
	if e != nil {
		return nil, e
	}
	creates, updates := 0, 0
	for code, row := range seen {
		if existing[code] {
			if mode == "create_only" {
				issues = append(issues, ImportIssue{Row: row, Code: "CODE_EXISTS", Message: "code already exists"})
			} else {
				updates++
			}
		} else {
			creates++
		}
	}
	summary := ImportSummary{Total: len(seen), Creates: creates, Updates: updates, Errors: len(issues)}
	preview := &ImportPreview{Summary: summary, Issues: issues}
	if len(issues) > 0 {
		return preview, nil
	}
	rows, _ := json.Marshal(map[string]any{"services": services, "medications": meds})
	sum, _ := json.Marshal(summary)
	id, e := utility.GenerateUUID()
	if e != nil {
		return nil, e
	}
	expiry := time.Now().UTC().Add(24 * time.Hour)
	hash := sha256.Sum256(rows)
	checksum := hex.EncodeToString(hash[:])
	imp := CatalogueImport{ID: id, Catalogue: kind, Mode: mode, Status: "validated", Rows: rows, Summary: sum, ExpiresAt: expiry, FileChecksum: checksum}
	if e = s.repo.SaveCatalogueImport(c, imp, actor); e != nil {
		return nil, e
	}
	preview.ImportID = id
	preview.ExpiresAt = &expiry
	s.recordCatalogueAudit(c, actor, "catalogue_import_previewed", id, map[string]any{"catalogue": kind, "rows": summary.Total, "mode": mode})
	return preview, nil
}

func (s *Service) CommitCatalogueImport(c context.Context, actor, kind, id string) (*ImportSummary, error) {
	x, e := s.repo.GetCatalogueImport(c, id)
	if e != nil {
		return nil, e
	}
	if x.Catalogue != kind || x.Status != "validated" || time.Now().After(x.ExpiresAt) {
		return nil, ErrConflict
	}
	var rows struct {
		Services    []CatalogueService     `json:"services"`
		Medications []MedicationDefinition `json:"medications"`
	}
	if e = json.Unmarshal(x.Rows, &rows); e != nil {
		return nil, e
	}
	if e = s.repo.CommitCatalogueImport(c, *x, rows.Services, rows.Medications); e != nil {
		return nil, e
	}
	var summary ImportSummary
	if e = json.Unmarshal(x.Summary, &summary); e != nil {
		return nil, e
	}
	s.recordCatalogueAudit(c, actor, "catalogue_import_committed", id, map[string]any{"catalogue": x.Catalogue, "rows": summary.Total, "mode": x.Mode})
	return &summary, nil
}
