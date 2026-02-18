SELECT
  id,
  content,
  created_at
FROM memory
WHERE created_at < ?1
ORDER BY RANDOM()
LIMIT ?2
