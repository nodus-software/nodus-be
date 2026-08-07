-- Replace implementation-oriented frequency codes with clinical terms for
-- every tenant, then restore forced row-level security.
ALTER TABLE prescription_frequencies NO FORCE ROW LEVEL SECURITY;

UPDATE prescription_frequencies
SET code = mapped.code, name = mapped.name
FROM (VALUES
  ('once_daily', 'OD', 'Once daily'),
  ('twice_daily', 'TD', 'Twice daily'),
  ('three_times_daily', 'TDS', 'Three times daily'),
  ('four_times_daily', 'QDS', 'Four times daily'),
  ('every_4_hours', 'Q4H', 'Every 4 hours'),
  ('every_6_hours', 'Q6H', 'Every 6 hours'),
  ('every_8_hours', 'Q8H', 'Every 8 hours'),
  ('every_12_hours', 'Q12H', 'Every 12 hours'),
  ('at_night', 'NOCTE', 'At night'),
  ('as_needed', 'PRN', 'As needed')
) AS mapped(old_code, code, name)
WHERE lower(trim(prescription_frequencies.code)) = mapped.old_code;

INSERT INTO prescription_frequencies (id, tenant_id, code, name)
SELECT gen_random_uuid(), o.id, 'STAT', 'Immediately'
FROM organizations o
WHERE NOT EXISTS (
  SELECT 1 FROM prescription_frequencies f
  WHERE f.tenant_id = o.id AND lower(trim(f.code)) = 'stat'
);

ALTER TABLE prescription_frequencies FORCE ROW LEVEL SECURITY;
