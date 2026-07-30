DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE code IN ('clinical:read','clinical:write','queues:manage','facilities:manage'));
DELETE FROM permissions WHERE code IN ('clinical:read','clinical:write','queues:manage','facilities:manage');
DROP TABLE IF EXISTS clinical_outbox, queue_routing_rules, queue_entry_history, queue_entries, clinical_notes,
  clinical_diagnoses, clinical_observations, clinical_encounters, clinical_visits, queues, beds, rooms, wards,
  service_points, departments, terminology_concepts, terminology_releases;
DROP TYPE IF EXISTS outbox_status, diagnosis_kind, queue_subject_type, queue_entry_status,
  clinical_encounter_status, clinical_encounter_type, clinical_visit_status, clinical_visit_type;
