INSERT INTO task_assessment (
  id,
  task_id,
  activation_id,
  effectiveness,
  expected_duration_seconds,
  tool_feedback,
  skill_feedback,
  suggestions,
  created_at
)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, NOW_ISO8601_MS())
