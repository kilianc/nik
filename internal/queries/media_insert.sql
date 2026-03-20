-- ?1: id, ?2: mime_type, ?3: size_bytes, ?4: local_path
-- ?5: describe_text, ?6: transcript_text, ?7: described_at, ?8: transcribed_at
INSERT INTO media (
  id,
  mime_type,
  size_bytes,
  local_path,
  describe_text,
  transcript_text,
  described_at,
  transcribed_at,
  created_at,
  updated_at
)
VALUES (
  ?1,
  ?2,
  ?3,
  ?4,
  ?5,
  ?6,
  NULLABLE_ISO8601_MS(?7),
  NULLABLE_ISO8601_MS(?8),
  NOW_ISO8601_MS(),
  NOW_ISO8601_MS()
);
