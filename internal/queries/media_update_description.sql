-- ?1: describe_text, ?2: described_at, ?3: id
UPDATE media
SET
  describe_text = ?1,
  described_at = ?2,
  updated_at = datetime('now')
WHERE id = ?3;
