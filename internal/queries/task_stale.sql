SELECT t.id
FROM task t
WHERE (
  t.status = 'running'
  AND t.started_at < ISO8601_MS(?1)
  AND (t.last_report_at IS NULL OR t.last_report_at < ISO8601_MS(?1))
)
OR (
  t.status = 'pending'
  AND t.created_at < ISO8601_MS(?1)
  AND (t.last_report_at IS NULL OR t.last_report_at < ISO8601_MS(?1))
)

