-- ?1: transcript_text, ?2: transcribed_at, ?3: id
UPDATE media
SET
  transcript_text = ?1,
  transcribed_at = ISO8601_MS(?2),
  updated_at = NOW_ISO8601_MS()
WHERE id = ?3;
