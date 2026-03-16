SELECT t.id
FROM task t
WHERE t.status = 'running'
  AND t.started_at < ?1
  AND (t.last_report_at IS NULL OR t.last_report_at < ?1)
