SELECT t.id
FROM task t
WHERE t.status = 'running'
  AND t.started_at < ?1
  AND (
    NOT EXISTS (SELECT 1 FROM task_report tr WHERE tr.task_id = t.id)
    OR (SELECT MAX(tr.created_at) FROM task_report tr WHERE tr.task_id = t.id) < ?1
  )
