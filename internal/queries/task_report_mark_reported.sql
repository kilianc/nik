UPDATE task_report
SET reported_at = datetime('now')
WHERE id = ?1
