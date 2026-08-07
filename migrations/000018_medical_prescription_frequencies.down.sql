ALTER TABLE prescription_frequencies NO FORCE ROW LEVEL SECURITY;

DELETE FROM prescription_frequencies WHERE lower(trim(code)) = 'stat';

UPDATE prescription_frequencies
SET code = mapped.code, name = mapped.name
FROM (VALUES
  ('od', 'once_daily', 'Once daily'),
  ('td', 'twice_daily', 'Twice daily'),
  ('tds', 'three_times_daily', 'Three times daily'),
  ('qds', 'four_times_daily', 'Four times daily'),
  ('q4h', 'every_4_hours', 'Every 4 hours'),
  ('q6h', 'every_6_hours', 'Every 6 hours'),
  ('q8h', 'every_8_hours', 'Every 8 hours'),
  ('q12h', 'every_12_hours', 'Every 12 hours'),
  ('nocte', 'at_night', 'At night'),
  ('prn', 'as_needed', 'As needed')
) AS mapped(old_code, code, name)
WHERE lower(trim(prescription_frequencies.code)) = mapped.old_code;

ALTER TABLE prescription_frequencies FORCE ROW LEVEL SECURITY;
