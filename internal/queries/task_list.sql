SELECT
  id,
  goal,
  status,
  json_extract(meta, '$.conversation_id') AS conversation_id,
  created_at,
  started_at,
  completed_at
FROM task
WHERE status IN ('pending', 'running')
   OR (status IN ('completed', 'failed', 'cancelled') AND completed_at > datetime('now', ?1))
ORDER BY
  CASE WHEN status IN ('pending', 'running') THEN 0 ELSE 1 END,
  created_at DESC
LIMIT 20
