-- ?1: id, ?2: describe_text, ?3: described_at, ?4: transcript_text, ?5: transcribed_at
UPDATE media
SET
  describe_text = COALESCE(?2, describe_text),
  described_at = COALESCE(NULLABLE_ISO8601_MS(?3), described_at),
  transcript_text = COALESCE(?4, transcript_text),
  transcribed_at = COALESCE(NULLABLE_ISO8601_MS(?5), transcribed_at),
  updated_at = NOW_ISO8601_MS()
WHERE id = ?1
