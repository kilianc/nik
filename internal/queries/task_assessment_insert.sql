INSERT INTO task_assessment (
  id,
  task_id,
  effectiveness_score,
  effectiveness_feedback,
  expected_duration_seconds,
  duration_feedback,
  tool_feedback,
  skill_feedback,
  recommendations,
  created_at
)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, NOW_ISO8601_MS())
