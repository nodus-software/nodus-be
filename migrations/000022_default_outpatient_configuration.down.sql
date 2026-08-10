DROP INDEX IF EXISTS uq_active_routing_match_priority;

DELETE FROM queue_routing_rules
WHERE name IN (
  'Default outpatient check-in to triage',
  'Default completed triage to consultation',
  'Default laboratory order routing',
  'Default medication order routing',
  'Default reviewed order to consultation'
);

-- Operational queue entries and their configuration are intentionally retained
-- on rollback so migration rollback cannot erase patient workflow history.
