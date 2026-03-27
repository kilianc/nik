UPDATE activation
SET
  instructions = COALESCE(?2, instructions),
  tools = COALESCE(?3, tools),
  tool_schemas = COALESCE(?4, tool_schemas),
  reasoning_effort = COALESCE(NULLIF(?5, ''), reasoning_effort),
  verbosity = COALESCE(NULLIF(?6, ''), verbosity),
  input_tokens = ?7,
  output_tokens = ?8,
  total_tokens = ?9,
  cached_tokens = ?10,
  reasoning_tokens = ?11,
  cost_usd = ?12,
  round_count = ?13,
  max_input_tokens_per_round = ?14,
  max_total_tokens_per_round = ?15,
  tool_call_count = ?16,
  duration_ms = ?17,
  error = ?18
WHERE id = ?1
