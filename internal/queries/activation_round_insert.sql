INSERT INTO activation_round (
  id,
  activation_id,
  round,
  messages,
  reasoning_summaries,
  input_tokens,
  output_tokens,
  cached_tokens,
  reasoning_tokens,
  created_at
)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, NOW_ISO8601_MS())
