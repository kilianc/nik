INSERT OR IGNORE INTO activation (
  id,
  source,
  source_id,
  model,
  reasoning_effort,
  input_tokens,
  output_tokens,
  total_tokens,
  cached_tokens,
  reasoning_tokens,
  cost_usd,
  tool_call_count,
  duration_ms,
  error,
  created_at
)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15)
