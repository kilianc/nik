SELECT
  id,
  goal,
  status,
  created_at
FROM task
WHERE json_extract(meta, '$.conversation_id') = ?1
  AND status IN ('pending', 'running')
ORDER BY created_at DESC
