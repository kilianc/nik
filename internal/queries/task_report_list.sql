SELECT
  tr.id,
  t.id AS task_id,
  tr.content,
  tr.created_at,
  t.goal,
  tr.status
FROM task_report tr
JOIN task t ON t.id = tr.task_id
WHERE t.conversation_id = ?1
  AND tr.created_at >= ?2
ORDER BY tr.created_at
