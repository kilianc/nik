SELECT
  tr.id,
  tr.task_id,
  tr.kind,
  tr.content,
  tr.reported_at,
  tr.created_at,
  t.meta,
  t.goal,
  t.status
FROM task_report tr
JOIN task t ON tr.task_id = t.id
WHERE tr.reported_at IS NULL
  AND t.status != 'cancelled'
ORDER BY tr.created_at ASC
