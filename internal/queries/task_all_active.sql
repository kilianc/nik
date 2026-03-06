SELECT
  id,
  goal,
  status,
  json_extract(meta, '$.conversation_id') AS conversation_id,
  retry_number,
  created_at
FROM task
WHERE status IN ('pending', 'running')
ORDER BY created_at DESC
