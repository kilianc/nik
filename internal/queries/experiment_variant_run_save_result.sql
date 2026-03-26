UPDATE experiment_variant_run
SET
  tool_calls = ?2,
  model_output = ?3,
  reasoning_summaries = ?4,
  input_tokens = ?5,
  output_tokens = ?6,
  cached_tokens = ?7,
  reasoning_tokens = ?8
WHERE id = ?1
