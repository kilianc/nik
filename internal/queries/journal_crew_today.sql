SELECT
  cm.id,
  cm.name,
  cm.prompt,
  cm.created_at,
  (SELECT COUNT(*) FROM task t WHERE t.crew_member_id = cm.id AND t.created_at >= ?1 AND t.created_at < ?2) AS task_count
FROM crew_member cm
WHERE cm.created_at >= ?1
  AND cm.created_at < ?2
ORDER BY cm.created_at
