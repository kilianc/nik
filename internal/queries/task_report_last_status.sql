SELECT status
FROM task_report
WHERE task_id = ?1
ORDER BY created_at DESC
LIMIT 1
