-- A small, usable outpatient workflow for every organization. These records are
-- ordinary tenant-owned configuration and can be renamed, deactivated, or
-- replaced by an administrator after installation.
INSERT INTO departments (id, tenant_id, code, name, description)
SELECT gen_random_uuid(), o.id, v.code, v.name, v.description
FROM organizations o
CROSS JOIN (VALUES
  ('OPD', 'Outpatient Department', 'Default outpatient clinical services'),
  ('LAB', 'Laboratory Department', 'Default diagnostic laboratory services'),
  ('PHARM', 'Pharmacy Department', 'Default medication dispensing services')
) AS v(code, name, description)
ON CONFLICT (tenant_id, code) DO NOTHING;

INSERT INTO service_points (id, tenant_id, department_id, code, name, kind)
SELECT gen_random_uuid(), o.id, d.id, v.code, v.name, v.kind
FROM organizations o
JOIN (VALUES
  ('OPD', 'OPD-TRIAGE', 'Outpatient Triage', 'triage'),
  ('OPD', 'OPD-CONSULT', 'General Consultation', 'consultation'),
  ('LAB', 'LAB-MAIN', 'Main Laboratory', 'laboratory'),
  ('PHARM', 'PHARM-MAIN', 'Main Pharmacy', 'pharmacy')
) AS v(department_code, code, name, kind) ON true
JOIN departments d ON d.tenant_id = o.id AND d.code = v.department_code
ON CONFLICT (tenant_id, code) DO NOTHING;

INSERT INTO queues (id, tenant_id, service_point_id, code, name)
SELECT gen_random_uuid(), o.id, sp.id, v.code, v.name
FROM organizations o
JOIN (VALUES
  ('OPD-TRIAGE', 'OPD-TRIAGE-Q', 'Outpatient Triage Queue'),
  ('OPD-CONSULT', 'OPD-CONSULT-Q', 'General Consultation Queue'),
  ('LAB-MAIN', 'LAB-Q', 'Laboratory Orders Queue'),
  ('PHARM-MAIN', 'PHARM-Q', 'Prescription Queue')
) AS v(service_point_code, code, name) ON true
JOIN service_points sp ON sp.tenant_id = o.id AND sp.code = v.service_point_code
ON CONFLICT (tenant_id, code) DO NOTHING;

INSERT INTO queue_routing_rules
  (id, tenant_id, name, event_type, visit_type, encounter_type, order_kind, service_category, target_queue_id, priority)
SELECT gen_random_uuid(), o.id, v.name, v.event_type, v.visit_type::clinical_visit_type,
       v.encounter_type::clinical_encounter_type, v.order_kind, v.service_category, q.id, v.priority
FROM organizations o
JOIN (VALUES
  ('Default outpatient check-in to triage', 'visit.created', 'outpatient', NULL, NULL, NULL, 'OPD-TRIAGE-Q', 0::smallint),
  ('Default completed triage to consultation', 'encounter.completed', 'outpatient', 'triage', NULL, NULL, 'OPD-CONSULT-Q', 0::smallint),
  ('Default laboratory order routing', 'order.created', NULL, NULL, 'service', 'laboratory', 'LAB-Q', 0::smallint),
  ('Default medication order routing', 'order.created', NULL, NULL, 'medication', 'pharmacy', 'PHARM-Q', 0::smallint),
  ('Default reviewed order to consultation', 'order.review_ready', 'outpatient', NULL, NULL, NULL, 'OPD-CONSULT-Q', 0::smallint)
) AS v(name, event_type, visit_type, encounter_type, order_kind, service_category, queue_code, priority) ON true
JOIN queues q ON q.tenant_id = o.id AND q.code = v.queue_code
ON CONFLICT (tenant_id, name) DO NOTHING;

-- Two equally specific rules at the same priority would make the destination
-- arbitrary. More specific rules and different priorities remain supported.
CREATE UNIQUE INDEX uq_active_routing_match_priority
ON queue_routing_rules (
  tenant_id,
  event_type,
  visit_type,
  encounter_type,
  order_kind,
  service_category,
  priority
)
NULLS NOT DISTINCT
WHERE active;

-- Recover active outpatient visits created while an organization had no
-- routing configuration. Existing active queue entries are never duplicated.
WITH inserted AS (
  INSERT INTO queue_entries
    (id, tenant_id, queue_id, subject_type, subject_id, patient_id, status, priority)
  SELECT gen_random_uuid(), v.tenant_id, q.id, 'visit', v.id, v.patient_id, 'waiting', 0
  FROM clinical_visits v
  JOIN queues q ON q.tenant_id = v.tenant_id AND q.code = 'OPD-TRIAGE-Q'
  WHERE v.visit_type = 'outpatient'
    AND v.status = 'active'
    AND NOT EXISTS (
      SELECT 1 FROM queue_entries e
      WHERE e.tenant_id = v.tenant_id
        AND e.subject_type = 'visit'
        AND e.subject_id = v.id
        AND e.status IN ('waiting', 'called', 'in_service', 'paused')
    )
  ON CONFLICT DO NOTHING
  RETURNING id, tenant_id, queue_id
)
INSERT INTO queue_entry_history
  (id, tenant_id, queue_entry_id, to_status, to_queue_id, automated, reason)
SELECT gen_random_uuid(), tenant_id, id, 'waiting', queue_id, true, 'Default outpatient routing backfill'
FROM inserted;
