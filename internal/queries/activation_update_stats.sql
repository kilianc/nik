UPDATE activation
SET input_tokens = ?2,
    output_tokens = ?3,
    total_tokens = ?4,
    cached_tokens = ?5,
    reasoning_tokens = ?6,
    cost_usd = ?7,
    tool_call_count = ?8,
    duration_ms = ?9
WHERE id = ?1
