INSERT INTO task_report (
  id,
  task_id,
  status,
  content,
  created_at
)
VALUES (?1, ?2, ?3, ?4, ISO8601_MS(?5))

