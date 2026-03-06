SELECT
  id,
  goal,
  status,
  json_extract(meta, '$.conversation_id') AS conversation_id,
  retry_number,
  created_at
FROM task
WHERE (retry_for_task_id = ?1 OR id = ?1)
  AND status IN ('pending', 'running')
