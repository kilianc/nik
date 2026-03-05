UPDATE activation
SET reasoning_effort = COALESCE(NULLIF(?2, ''), reasoning_effort),
    input_tokens = input_tokens + ?3,
    output_tokens = output_tokens + ?4,
    total_tokens = total_tokens + ?5,
    cached_tokens = cached_tokens + ?6,
    reasoning_tokens = reasoning_tokens + ?7,
    cost_usd = cost_usd + ?8,
    tool_call_count = tool_call_count + ?9,
    duration_ms = duration_ms + ?10,
    error = ?11
WHERE id = ?1
