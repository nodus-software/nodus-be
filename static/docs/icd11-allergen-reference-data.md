# ICD-11 and allergen reference data

## ICD-11 operator import

ICD-11 releases are global platform data. Tenant administrators may enable or
disable concepts for their facility, but cannot upload or edit WHO content.

Download the English ICD-11 MMS **Simple Tabulation** workbook from WHO and
validate it without changing the database:

```sh
MIGRATION_DB_URL='postgres://…' make import-icd11 FILE=/secure/path/SimpleTabulation-ICD-11-MMS-en.xlsx
```

After reviewing the reported version, row count and SHA-256 checksum, commit it:

```sh
MIGRATION_DB_URL='postgres://…' make import-icd11 FILE=/secure/path/SimpleTabulation-ICD-11-MMS-en.xlsx COMMIT=1
```

The import is atomic and retains older releases for historical diagnoses. A
repeat with the same version and checksum is idempotent; the same version with a
different checksum is rejected. Only `category` rows marked `Primary
tabulation=True` are selectable as standalone diagnoses. Postcoordination and
extension codes are intentionally outside this release.

Source attribution: *International Classification of Diseases, Eleventh
Revision (ICD-11), World Health Organization (WHO) 2019.* Licensed under CC
BY-ND 3.0 IGO. The workbook is an external operator artifact and must not be
committed to this repository.

## Allergen starter catalogue

Every tenant receives an editable starter catalogue covering common medication
classes and ingredients, foods, contrast media, latex/contact substances,
insect venom and environmental allergens. It is practical reference data—not
an official Kenyan national allergen standard. The medication choices and
aliases are informed by Kenyan essential-medicine usage and Pharmacy and
Poisons Board pharmacovigilance reports.

Deactivation prevents new selection but never changes historical patient
records. Clinicians may use **Other allergen** when the configured catalogue is
insufficient; that path is explicitly marked as custom and audited.
