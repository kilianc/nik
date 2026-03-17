-- ?1: id (UUID string)
-- ?2: field name
-- ?3: scalar value (TEXT) — used for name, notes, one_liner, timezone, location
-- ?4: JSON array value (TEXT) — used for nicknames, emails, phone_numbers
UPDATE contact
SET
  name = CASE WHEN ?2 = 'name' THEN ?3 ELSE name END,
  notes = CASE WHEN ?2 = 'notes' THEN ?3 ELSE notes END,
  one_liner = CASE WHEN ?2 = 'one_liner' THEN ?3 ELSE one_liner END,
  timezone = CASE WHEN ?2 = 'timezone' THEN ?3 ELSE timezone END,
  location = CASE WHEN ?2 = 'location' THEN ?3 ELSE location END,
  nicknames = CASE WHEN ?2 = 'nicknames' THEN ?4 ELSE nicknames END,
  emails = CASE WHEN ?2 = 'emails' THEN ?4 ELSE emails END,
  phone_numbers = CASE WHEN ?2 = 'phone_numbers' THEN ?4 ELSE phone_numbers END,
  updated_at = NOW_ISO8601_MS()
WHERE id = ?1;
