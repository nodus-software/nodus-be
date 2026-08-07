-- Dropping the vocabularies leaves medication_catalogue.dosage_form, .route and
-- .unit_of_measure as free text again. The normalisation the up migration
-- performed is not reversible: values that had no vocabulary entry were cleared,
-- and abbreviations were rewritten to their canonical code.

DROP TABLE IF EXISTS units_of_measure;
DROP TABLE IF EXISTS administration_routes;
DROP TABLE IF EXISTS medication_dosage_forms;
