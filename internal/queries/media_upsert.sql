-- ?1: id hash, ?2: mime_type, ?3: size_bytes, ?4: local_path
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
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, datetime('now'), datetime('now'))
ON CONFLICT (id) DO UPDATE SET
  mime_type = COALESCE(excluded.mime_type, media.mime_type),
  size_bytes = COALESCE(excluded.size_bytes, media.size_bytes),
  local_path = COALESCE(excluded.local_path, media.local_path),
  describe_text = COALESCE(excluded.describe_text, media.describe_text),
  transcript_text = COALESCE(excluded.transcript_text, media.transcript_text),
  described_at = COALESCE(excluded.described_at, media.described_at),
  transcribed_at = COALESCE(excluded.transcribed_at, media.transcribed_at),
  updated_at = datetime('now');
