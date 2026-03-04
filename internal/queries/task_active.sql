SELECT
  id,
  goal,
  status,
  created_at
FROM task
WHERE source = ?1
  AND source_id = ?2
  AND status IN ('pending', 'running')
ORDER BY created_at DESC
