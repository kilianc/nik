SELECT
  t.id,
  t.goal,
  t.retry_for_task_id,
  t.retry_number,
  cm.name AS crew_member_name,
  t.created_at
FROM task t
LEFT JOIN crew_member cm ON cm.id = t.crew_member_id
WHERE t.conversation_id = ?1
  AND t.created_at >= ?2
ORDER BY t.created_at
