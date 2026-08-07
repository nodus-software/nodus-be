package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"nodus-health/internal/clinical"
	"nodus-health/internal/platform/db"
)

const serviceCatalogueCols = `id,reference_item_id,code,name,category,department_id,service_point_id,price_minor,currency,requires_order,requires_result,estimated_duration_minutes,active,created_at,updated_at`
const medicationCatalogueCols = `id,reference_item_id,code,generic_name,brand_name,strength,dosage_form,route,pack_size,unit_of_measure,prescription_required,active,created_at,updated_at`

func scanCatalogueService(row pgx.Row) (*clinical.CatalogueService, error) {
	var x clinical.CatalogueService
	var minor *int64
	err := row.Scan(&x.ID, &x.ReferenceItemID, &x.Code, &x.Name, &x.Category, &x.DepartmentID, &x.ServicePointID, &minor, &x.Currency, &x.RequiresOrder, &x.RequiresResult, &x.EstimatedDurationMinutes, &x.Active, &x.CreatedAt, &x.UpdatedAt)
	if minor != nil {
		price := float64(*minor) / 100
		x.Price = &price
	}
	return &x, normalizeResourceError(err)
}

func scanMedication(row pgx.Row) (*clinical.MedicationDefinition, error) {
	var x clinical.MedicationDefinition
	err := row.Scan(&x.ID, &x.ReferenceItemID, &x.Code, &x.GenericName, &x.BrandName, &x.Strength, &x.DosageForm, &x.Route, &x.PackSize, &x.UnitOfMeasure, &x.PrescriptionRequired, &x.Active, &x.CreatedAt, &x.UpdatedAt)
	return &x, normalizeResourceError(err)
}

func cataloguePaging(f clinical.CatalogueFilters) (int, int) {
	page, perPage := f.Page, f.PerPage
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}

func (r *Repository) ListCatalogueServices(c context.Context, f clinical.CatalogueFilters) ([]clinical.CatalogueService, int, error) {
	where := []string{"1=1"}
	args := []any{}
	add := func(sql string, value any) {
		args = append(args, value)
		n := len(args)
		where = append(where, fmt.Sprintf(sql, n, n, n))
	}
	if f.Query != "" {
		add("(code ILIKE '%%' || $%d || '%%' OR name ILIKE '%%' || $%d || '%%')", f.Query)
	}
	if f.Status == "active" {
		where = append(where, "active")
	} else if f.Status == "inactive" {
		where = append(where, "NOT active")
	}
	if f.Category != "" && f.Category != "all" {
		add("category=$%d", f.Category)
	}
	if f.DepartmentID != "" && f.DepartmentID != "all" {
		add("department_id=$%d", f.DepartmentID)
	}
	w := strings.Join(where, " AND ")
	var total int
	if err := r.exec(c).QueryRow(c, "SELECT count(*) FROM clinical_services WHERE "+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	page, perPage := cataloguePaging(f)
	args = append(args, perPage, (page-1)*perPage)
	rows, err := r.exec(c).Query(c, "SELECT "+serviceCatalogueCols+" FROM clinical_services WHERE "+w+fmt.Sprintf(" ORDER BY name LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []clinical.CatalogueService{}
	for rows.Next() {
		x, e := scanCatalogueService(rows)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, *x)
	}
	return out, total, rows.Err()
}

func priceMinor(price *float64) *int64 {
	if price == nil {
		return nil
	}
	v := int64(*price*100 + .5)
	return &v
}

func (r *Repository) CreateCatalogueService(c context.Context, x clinical.CatalogueService) (*clinical.CatalogueService, error) {
	if err := r.validateCatalogueServiceLocations(c, x.DepartmentID, x.ServicePointID); err != nil {
		return nil, err
	}
	return scanCatalogueService(r.exec(c).QueryRow(c, `INSERT INTO clinical_services(id,reference_item_id,code,name,category,department_id,service_point_id,price_minor,currency,requires_order,requires_result,estimated_duration_minutes,active)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING `+serviceCatalogueCols,
		x.ID, x.ReferenceItemID, x.Code, x.Name, x.Category, x.DepartmentID, x.ServicePointID, priceMinor(x.Price), x.Currency, x.RequiresOrder, x.RequiresResult, x.EstimatedDurationMinutes, x.Active))
}

func (r *Repository) UpdateCatalogueService(c context.Context, id string, x clinical.CatalogueService) (*clinical.CatalogueService, error) {
	if err := r.validateCatalogueServiceLocations(c, x.DepartmentID, x.ServicePointID); err != nil {
		return nil, err
	}
	return scanCatalogueService(r.exec(c).QueryRow(c, `UPDATE clinical_services SET reference_item_id=$2,code=$3,name=$4,category=$5,department_id=$6,service_point_id=$7,price_minor=$8,currency=$9,requires_order=$10,requires_result=$11,estimated_duration_minutes=$12 WHERE id=$1 RETURNING `+serviceCatalogueCols,
		id, x.ReferenceItemID, x.Code, x.Name, x.Category, x.DepartmentID, x.ServicePointID, priceMinor(x.Price), x.Currency, x.RequiresOrder, x.RequiresResult, x.EstimatedDurationMinutes))
}

func (r *Repository) validateCatalogueServiceLocations(c context.Context, departmentID, servicePointID *string) error {
	if servicePointID == nil {
		return nil
	}
	var valid bool
	err := r.exec(c).QueryRow(c, `SELECT sp.active AND d.active AND ($2::uuid IS NULL OR sp.department_id=$2) FROM service_points sp JOIN departments d ON d.id=sp.department_id WHERE sp.id=$1`, *servicePointID, departmentID).Scan(&valid)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !valid {
		return clinical.ErrInvalidInput
	}
	return err
}

func (r *Repository) SetCatalogueServiceActive(c context.Context, id string, active bool) (*clinical.CatalogueService, error) {
	return scanCatalogueService(r.exec(c).QueryRow(c, "UPDATE clinical_services SET active=$2 WHERE id=$1 RETURNING "+serviceCatalogueCols, id, active))
}

func (r *Repository) ListMedicationCatalogue(c context.Context, f clinical.CatalogueFilters) ([]clinical.MedicationDefinition, int, error) {
	where := []string{"1=1"}
	args := []any{}
	add := func(sql string, value any) {
		args = append(args, value)
		n := len(args)
		where = append(where, fmt.Sprintf(sql, n, n, n))
	}
	if f.Query != "" {
		add("(code ILIKE '%%' || $%d || '%%' OR generic_name ILIKE '%%' || $%d || '%%' OR COALESCE(brand_name,'') ILIKE '%%' || $%d || '%%')", f.Query)
	}
	if f.Status == "active" {
		where = append(where, "active")
	} else if f.Status == "inactive" {
		where = append(where, "NOT active")
	}
	if f.Prescription == "required" {
		where = append(where, "prescription_required")
	} else if f.Prescription == "otc" {
		where = append(where, "NOT prescription_required")
	}
	w := strings.Join(where, " AND ")
	var total int
	if err := r.exec(c).QueryRow(c, "SELECT count(*) FROM medication_catalogue WHERE "+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	page, perPage := cataloguePaging(f)
	args = append(args, perPage, (page-1)*perPage)
	rows, err := r.exec(c).Query(c, "SELECT "+medicationCatalogueCols+" FROM medication_catalogue WHERE "+w+fmt.Sprintf(" ORDER BY generic_name LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []clinical.MedicationDefinition{}
	for rows.Next() {
		x, e := scanMedication(rows)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, *x)
	}
	return out, total, rows.Err()
}

func (r *Repository) CreateMedicationDefinition(c context.Context, x clinical.MedicationDefinition) (*clinical.MedicationDefinition, error) {
	return scanMedication(r.exec(c).QueryRow(c, `INSERT INTO medication_catalogue(id,reference_item_id,code,generic_name,brand_name,strength,dosage_form,route,pack_size,unit_of_measure,prescription_required,active)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING `+medicationCatalogueCols, x.ID, x.ReferenceItemID, x.Code, x.GenericName, x.BrandName, x.Strength, x.DosageForm, x.Route, x.PackSize, x.UnitOfMeasure, x.PrescriptionRequired, x.Active))
}

func (r *Repository) UpdateMedicationDefinition(c context.Context, id string, x clinical.MedicationDefinition) (*clinical.MedicationDefinition, error) {
	return scanMedication(r.exec(c).QueryRow(c, `UPDATE medication_catalogue SET reference_item_id=$2,code=$3,generic_name=$4,brand_name=$5,strength=$6,dosage_form=$7,route=$8,pack_size=$9,unit_of_measure=$10,prescription_required=$11 WHERE id=$1 RETURNING `+medicationCatalogueCols, id, x.ReferenceItemID, x.Code, x.GenericName, x.BrandName, x.Strength, x.DosageForm, x.Route, x.PackSize, x.UnitOfMeasure, x.PrescriptionRequired))
}

func (r *Repository) SetMedicationDefinitionActive(c context.Context, id string, active bool) (*clinical.MedicationDefinition, error) {
	return scanMedication(r.exec(c).QueryRow(c, "UPDATE medication_catalogue SET active=$2 WHERE id=$1 RETURNING "+medicationCatalogueCols, id, active))
}

func (r *Repository) ListCatalogueReferences(c context.Context, kind string, f clinical.CatalogueFilters) ([]clinical.CatalogueReferenceItem, int, error) {
	page, perPage := cataloguePaging(f)
	offset := (page - 1) * perPage
	var countSQL, query string
	if kind == "services" {
		countSQL = `SELECT count(*) FROM service_reference_items i JOIN catalogue_reference_releases r ON r.id=i.release_id WHERE r.active AND i.active AND ($1='' OR i.searchable_text ILIKE '%'||$1||'%') AND ($2='' OR $2='all' OR i.category=$2)`
		query = `SELECT i.id,r.source_system,r.version,i.source_code,i.name,i.category,NULL::text,NULL::text,NULL::text,i.standard_system,i.standard_code FROM service_reference_items i JOIN catalogue_reference_releases r ON r.id=i.release_id WHERE r.active AND i.active AND ($1='' OR i.searchable_text ILIKE '%'||$1||'%') AND ($2='' OR $2='all' OR i.category=$2) ORDER BY i.name LIMIT $3 OFFSET $4`
	} else if kind == "medications" {
		countSQL = `SELECT count(*) FROM medication_reference_items i JOIN catalogue_reference_releases r ON r.id=i.release_id WHERE r.active AND i.active AND ($1='' OR i.searchable_text ILIKE '%'||$1||'%') AND ($2='' OR $2<>'')`
		query = `SELECT i.id,r.source_system,r.version,i.source_code,i.generic_name,NULL::text,i.strength,i.dosage_form,i.route,i.standard_system,i.standard_code FROM medication_reference_items i JOIN catalogue_reference_releases r ON r.id=i.release_id WHERE r.active AND i.active AND ($1='' OR i.searchable_text ILIKE '%'||$1||'%') AND ($2='' OR $2<>'') ORDER BY i.generic_name LIMIT $3 OFFSET $4`
	} else {
		return nil, 0, clinical.ErrInvalidInput
	}
	var total int
	category := f.Category
	if kind == "medications" {
		category = ""
	}
	if err := r.exec(c).QueryRow(c, countSQL, f.Query, category).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.exec(c).Query(c, query, f.Query, category, perPage, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []clinical.CatalogueReferenceItem{}
	for rows.Next() {
		var x clinical.CatalogueReferenceItem
		if err = rows.Scan(&x.ID, &x.SourceSystem, &x.SourceVersion, &x.SourceCode, &x.Name, &x.Category, &x.Strength, &x.DosageForm, &x.Route, &x.StandardSystem, &x.StandardCode); err != nil {
			return nil, 0, err
		}
		out = append(out, x)
	}
	return out, total, rows.Err()
}

func rollbackSavepoint(c context.Context, exec db.DBTX) {
	_, _ = exec.Exec(c, "ROLLBACK TO SAVEPOINT catalogue_batch")
}

func (r *Repository) AdoptServiceReferences(c context.Context, items []clinical.CatalogueService) ([]clinical.CatalogueService, error) {
	exec := r.exec(c)
	if _, e := exec.Exec(c, "SAVEPOINT catalogue_batch"); e != nil {
		return nil, e
	}
	out := make([]clinical.CatalogueService, 0, len(items))
	for _, x := range items {
		var ref clinical.CatalogueReferenceItem
		e := exec.QueryRow(c, `SELECT i.id,r.source_system,r.version,i.source_code,i.name,i.category,NULL::text,NULL::text,NULL::text,i.standard_system,i.standard_code FROM service_reference_items i JOIN catalogue_reference_releases r ON r.id=i.release_id WHERE i.id=$1 AND i.active AND r.active`, *x.ReferenceItemID).Scan(&ref.ID, &ref.SourceSystem, &ref.SourceVersion, &ref.SourceCode, &ref.Name, &ref.Category, &ref.Strength, &ref.DosageForm, &ref.Route, &ref.StandardSystem, &ref.StandardCode)
		if e != nil {
			rollbackSavepoint(c, exec)
			return nil, normalizeResourceError(e)
		}
		x.Name = ref.Name
		x.Category = *ref.Category
		created, e := r.CreateCatalogueService(c, x)
		if e != nil {
			rollbackSavepoint(c, exec)
			return nil, e
		}
		out = append(out, *created)
	}
	_, _ = exec.Exec(c, "RELEASE SAVEPOINT catalogue_batch")
	return out, nil
}

func (r *Repository) AdoptMedicationReferences(c context.Context, items []clinical.MedicationDefinition) ([]clinical.MedicationDefinition, error) {
	exec := r.exec(c)
	if _, e := exec.Exec(c, "SAVEPOINT catalogue_batch"); e != nil {
		return nil, e
	}
	// Reference items carry whatever the upstream release said, so their dosage
	// form and route are mapped onto this facility's vocabularies on the way in.
	dosageForms, e := r.VocabularyCodes(c, clinical.DosageFormKind)
	if e != nil {
		return nil, e
	}
	routes, e := r.VocabularyCodes(c, clinical.RouteKind)
	if e != nil {
		return nil, e
	}
	out := make([]clinical.MedicationDefinition, 0, len(items))
	for _, x := range items {
		var ref clinical.CatalogueReferenceItem
		e := exec.QueryRow(c, `SELECT i.id,r.source_system,r.version,i.source_code,i.generic_name,NULL::text,i.strength,i.dosage_form,i.route,i.standard_system,i.standard_code FROM medication_reference_items i JOIN catalogue_reference_releases r ON r.id=i.release_id WHERE i.id=$1 AND i.active AND r.active`, *x.ReferenceItemID).Scan(&ref.ID, &ref.SourceSystem, &ref.SourceVersion, &ref.SourceCode, &ref.Name, &ref.Category, &ref.Strength, &ref.DosageForm, &ref.Route, &ref.StandardSystem, &ref.StandardCode)
		if e != nil {
			rollbackSavepoint(c, exec)
			return nil, normalizeResourceError(e)
		}
		x.GenericName = ref.Name
		x.Strength = ref.Strength
		x.DosageForm = clinical.ResolveVocabularyOrNil(ref.DosageForm, clinical.DosageFormKind, dosageForms)
		x.Route = clinical.ResolveVocabularyOrNil(ref.Route, clinical.RouteKind, routes)
		created, e := r.CreateMedicationDefinition(c, x)
		if e != nil {
			rollbackSavepoint(c, exec)
			return nil, e
		}
		out = append(out, *created)
	}
	_, _ = exec.Exec(c, "RELEASE SAVEPOINT catalogue_batch")
	return out, nil
}

func (r *Repository) CatalogueCodes(c context.Context, kind string, codes []string) (map[string]bool, error) {
	table := "clinical_services"
	if kind == "medications" {
		table = "medication_catalogue"
	} else if kind != "services" {
		return nil, clinical.ErrInvalidInput
	}
	rows, e := r.exec(c).Query(c, "SELECT lower(code) FROM "+table+" WHERE lower(code)=ANY($1)", codes)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var code string
		if e = rows.Scan(&code); e != nil {
			return nil, e
		}
		out[code] = true
	}
	return out, rows.Err()
}

// VocabularyCodes returns lower(code) -> stored code for the tenant's active
// entries in one of the prescribing reference lists. Inactive entries are left
// out so deactivating one stops new use without invalidating what already
// references it.
func (r *Repository) VocabularyCodes(c context.Context, kind string) (map[string]string, error) {
	table, _, e := resourceSpec(kind)
	if e != nil {
		return nil, e
	}
	rows, e := r.exec(c).Query(c, "SELECT lower(code),code FROM "+table+" WHERE active")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, code string
		if e = rows.Scan(&key, &code); e != nil {
			return nil, e
		}
		out[key] = code
	}
	return out, rows.Err()
}

func (r *Repository) ResolveDepartmentCodes(c context.Context, codes []string) (map[string]string, error) {
	rows, err := r.exec(c).Query(c, "SELECT lower(code),id FROM departments WHERE active AND lower(code)=ANY($1)", codes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var code, id string
		if err = rows.Scan(&code, &id); err != nil {
			return nil, err
		}
		out[code] = id
	}
	return out, rows.Err()
}

func (r *Repository) ResolveServicePointCodes(c context.Context, codes []string) (map[string]clinical.ServicePointImportReference, error) {
	rows, err := r.exec(c).Query(c, "SELECT lower(code),id,department_id FROM service_points WHERE active AND lower(code)=ANY($1)", codes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]clinical.ServicePointImportReference{}
	for rows.Next() {
		var code string
		var ref clinical.ServicePointImportReference
		if err = rows.Scan(&code, &ref.ID, &ref.DepartmentID); err != nil {
			return nil, err
		}
		out[code] = ref
	}
	return out, rows.Err()
}

func (r *Repository) ResolveCatalogueReferenceCodes(c context.Context, kind string, keys []string) (map[string]string, error) {
	var query string
	if kind == "services" {
		query = `SELECT lower(r.source_system)||'|'||lower(i.source_code),i.id FROM service_reference_items i JOIN catalogue_reference_releases r ON r.id=i.release_id WHERE r.active AND i.active AND lower(r.source_system)||'|'||lower(i.source_code)=ANY($1)`
	} else if kind == "medications" {
		query = `SELECT lower(r.source_system)||'|'||lower(i.source_code),i.id FROM medication_reference_items i JOIN catalogue_reference_releases r ON r.id=i.release_id WHERE r.active AND i.active AND lower(r.source_system)||'|'||lower(i.source_code)=ANY($1)`
	} else {
		return nil, clinical.ErrInvalidInput
	}
	rows, err := r.exec(c).Query(c, query, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, id string
		if err = rows.Scan(&key, &id); err != nil {
			return nil, err
		}
		out[key] = id
	}
	return out, rows.Err()
}

func (r *Repository) SaveCatalogueImport(c context.Context, x clinical.CatalogueImport, actor string) error {
	_, e := r.exec(c).Exec(c, `INSERT INTO catalogue_imports(id,catalogue,mode,file_name,file_checksum,rows,summary,created_by,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, x.ID, x.Catalogue, x.Mode, "catalogue.csv", x.FileChecksum, x.Rows, x.Summary, actor, x.ExpiresAt)
	return e
}
func (r *Repository) GetCatalogueImport(c context.Context, id string) (*clinical.CatalogueImport, error) {
	var x clinical.CatalogueImport
	e := r.exec(c).QueryRow(c, `SELECT id,catalogue::text,mode::text,status::text,rows,summary,expires_at FROM catalogue_imports WHERE id=$1`, id).Scan(&x.ID, &x.Catalogue, &x.Mode, &x.Status, &x.Rows, &x.Summary, &x.ExpiresAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, clinical.ErrNotFound
	}
	return &x, e
}

func (r *Repository) CommitCatalogueImport(c context.Context, x clinical.CatalogueImport, services []clinical.CatalogueService, meds []clinical.MedicationDefinition) error {
	exec := r.exec(c)
	if _, e := exec.Exec(c, "SAVEPOINT catalogue_batch"); e != nil {
		return e
	}
	var e error
	if x.Catalogue == "services" {
		for _, v := range services {
			if e = r.validateCatalogueServiceLocations(c, v.DepartmentID, v.ServicePointID); e != nil {
				break
			}
			conflict := ""
			if x.Mode == "upsert" {
				conflict = ` ON CONFLICT (tenant_id,(lower(code))) DO UPDATE SET reference_item_id=EXCLUDED.reference_item_id,name=EXCLUDED.name,category=EXCLUDED.category,department_id=EXCLUDED.department_id,service_point_id=EXCLUDED.service_point_id,price_minor=EXCLUDED.price_minor,currency=EXCLUDED.currency,requires_order=EXCLUDED.requires_order,requires_result=EXCLUDED.requires_result,estimated_duration_minutes=EXCLUDED.estimated_duration_minutes,active=EXCLUDED.active`
			}
			_, e = exec.Exec(c, `INSERT INTO clinical_services(id,reference_item_id,code,name,category,department_id,service_point_id,price_minor,currency,requires_order,requires_result,estimated_duration_minutes,active) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`+conflict, v.ID, v.ReferenceItemID, v.Code, v.Name, v.Category, v.DepartmentID, v.ServicePointID, priceMinor(v.Price), v.Currency, v.RequiresOrder, v.RequiresResult, v.EstimatedDurationMinutes, v.Active)
			if e != nil {
				break
			}
		}
	} else {
		for _, v := range meds {
			conflict := ""
			if x.Mode == "upsert" {
				conflict = ` ON CONFLICT (tenant_id,(lower(code))) DO UPDATE SET reference_item_id=EXCLUDED.reference_item_id,generic_name=EXCLUDED.generic_name,brand_name=EXCLUDED.brand_name,strength=EXCLUDED.strength,dosage_form=EXCLUDED.dosage_form,route=EXCLUDED.route,pack_size=EXCLUDED.pack_size,unit_of_measure=EXCLUDED.unit_of_measure,prescription_required=EXCLUDED.prescription_required,active=EXCLUDED.active`
			}
			_, e = exec.Exec(c, `INSERT INTO medication_catalogue(id,reference_item_id,code,generic_name,brand_name,strength,dosage_form,route,pack_size,unit_of_measure,prescription_required,active) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`+conflict, v.ID, v.ReferenceItemID, v.Code, v.GenericName, v.BrandName, v.Strength, v.DosageForm, v.Route, v.PackSize, v.UnitOfMeasure, v.PrescriptionRequired, v.Active)
			if e != nil {
				break
			}
		}
	}
	if e == nil {
		tag, err := exec.Exec(c, `UPDATE catalogue_imports SET status='committed',committed_at=now() WHERE id=$1 AND status='validated' AND expires_at>now()`, x.ID)
		e = err
		if e == nil && tag.RowsAffected() != 1 {
			e = clinical.ErrConflict
		}
	}
	if e != nil {
		rollbackSavepoint(c, exec)
		return normalizeResourceError(e)
	}
	_, _ = exec.Exec(c, "RELEASE SAVEPOINT catalogue_batch")
	return nil
}
