UPDATE activation
SET
  reasoning_effort = COALESCE(NULLIF(?2, ''), reasoning_effort),
  input_tokens = input_tokens + ?3,
  output_tokens = output_tokens + ?4,
  total_tokens = total_tokens + ?5,
  cached_tokens = cached_tokens + ?6,
  reasoning_tokens = reasoning_tokens + ?7,
  cost_usd = cost_usd + ?8,
  round_count = MAX(round_count, ?9),
  max_input_tokens_per_round = MAX(max_input_tokens_per_round, ?10),
  max_total_tokens_per_round = MAX(max_total_tokens_per_round, ?11),
  tool_call_count = tool_call_count + ?12,
  duration_ms = duration_ms + ?13,
  error = ?14,
  output = ?15
WHERE id = ?1
