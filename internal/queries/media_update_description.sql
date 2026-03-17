-- ?1: describe_text, ?2: described_at, ?3: id
UPDATE media
SET
  describe_text = ?1,
  described_at = ISO8601_MS(?2),
  updated_at = NOW_ISO8601_MS()
WHERE id = ?3;
