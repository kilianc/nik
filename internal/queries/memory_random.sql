SELECT
  id,
  content,
  created_at
FROM memory
WHERE created_at < ?1
  AND deleted_at IS NULL
ORDER BY RANDOM()
LIMIT ?2
