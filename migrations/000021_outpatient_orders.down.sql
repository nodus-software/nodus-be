DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE code IN ('outpatient:read','outpatient:complete','outpatient:workflow-override','orders:read','orders:place','orders:fulfill','orders:cancel','prescriptions:place','queues:view','queues:operate','queues:configure'));
DELETE FROM permissions WHERE code IN ('outpatient:read','outpatient:complete','outpatient:workflow-override','orders:read','orders:place','orders:fulfill','orders:cancel','prescriptions:place','queues:view','queues:operate','queues:configure');
DROP TABLE IF EXISTS clinical_medication_order_details, clinical_service_order_details, clinical_orders;
ALTER TABLE queue_routing_rules DROP COLUMN IF EXISTS service_category, DROP COLUMN IF EXISTS order_kind, DROP COLUMN IF EXISTS encounter_type;
ALTER TABLE clinical_visits DROP COLUMN IF EXISTS service_point_id;
