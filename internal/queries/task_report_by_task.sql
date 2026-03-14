SELECT
  id,
  status,
  content,
  created_at
FROM task_report
WHERE task_id = ?1
ORDER BY created_at ASC
