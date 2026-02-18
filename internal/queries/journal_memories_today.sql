-- ?1: day start (UTC), ?2: day end (UTC)
SELECT
  id,
  content,
  created_at
FROM memory
WHERE created_at >= datetime(?1) AND created_at < datetime(?2)
ORDER BY created_at ASC
