SELECT
  id,
  experiment_variant_id,
  tool_calls,
  model_output,
  reasoning_summaries,
  is_desired,
  input_tokens,
  output_tokens,
  cached_tokens,
  reasoning_tokens,
  created_at
FROM experiment_run
WHERE experiment_variant_id = ?1
ORDER BY created_at ASC
