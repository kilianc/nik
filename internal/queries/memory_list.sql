SELECT
  id,
  content,
  metadata,
  source,
  source_id,
  created_at
FROM memory
WHERE deleted_at IS NULL
ORDER BY created_at DESC
LIMIT ?1
