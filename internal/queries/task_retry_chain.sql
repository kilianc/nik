SELECT
  t.id,
  t.retry_number,
  t.goal,
  t.status,
  COALESCE(tr.content, '') AS report_content,
  tr.created_at
FROM task t
LEFT JOIN task_report tr ON tr.task_id = t.id
WHERE t.retry_for_task_id = ?1 OR t.id = ?1
ORDER BY t.retry_number, tr.created_at

