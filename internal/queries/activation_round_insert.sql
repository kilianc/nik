INSERT INTO activation_round (
  id,
  activation_id,
  round,
  user_input,
  model_output,
  reasoning_summaries,
  created_at
)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, NOW_ISO8601_MS())
