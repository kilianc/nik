INSERT INTO tool_call (
  id,
  activation_id,
  name,
  round,
  input,
  output,
  duration_ms,
  error,
  created_at
)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, NOW_ISO8601_MS())
