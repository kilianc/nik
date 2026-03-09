SELECT
  t.id,
  t.goal,
  t.retry_for_task_id,
  t.retry_number,
  t.created_at
FROM task t
WHERE t.conversation_id = ?1
  AND t.created_at >= ?2
ORDER BY t.created_at
