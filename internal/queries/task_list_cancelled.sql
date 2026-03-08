SELECT
  t.id,
  t.goal,
  t.completed_at
FROM task t
WHERE t.conversation_id = ?1
  AND t.status = 'cancelled'
  AND t.completed_at >= ?2
ORDER BY t.completed_at
