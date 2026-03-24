INSERT INTO experiment_run (
  id,
  experiment_variant_id,
  tool_calls,
  model_output,
  reasoning_summaries,
  is_desired,
  input_tokens,
  output_tokens,
  cached_tokens,
  reasoning_tokens
)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10)
