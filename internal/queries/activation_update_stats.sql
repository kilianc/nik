UPDATE activation
SET
  reasoning_effort = COALESCE(NULLIF(?2, ''), reasoning_effort),
  verbosity = COALESCE(NULLIF(?3, ''), verbosity),
  input_tokens = input_tokens + ?4,
  output_tokens = output_tokens + ?5,
  total_tokens = total_tokens + ?6,
  cached_tokens = cached_tokens + ?7,
  reasoning_tokens = reasoning_tokens + ?8,
  cost_usd = cost_usd + ?9,
  round_count = MAX(round_count, ?10),
  max_input_tokens_per_round = MAX(max_input_tokens_per_round, ?11),
  max_total_tokens_per_round = MAX(max_total_tokens_per_round, ?12),
  tool_call_count = tool_call_count + ?13,
  duration_ms = duration_ms + ?14,
  error = ?15
WHERE id = ?1
