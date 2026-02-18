SELECT
  id,
  content,
  metadata,
  source,
  source_id,
  created_at
FROM memory
ORDER BY created_at DESC
LIMIT ?1
