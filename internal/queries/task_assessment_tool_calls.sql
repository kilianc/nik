SELECT
  tc.name,
  COALESCE(ar.round, 0),
  tc.input,
  tc.output,
  tc.duration_ms,
  tc.error,
  tc.created_at
FROM tool_call tc
LEFT JOIN activation_round ar ON ar.id = tc.activation_round_id
WHERE tc.activation_id = ?1
ORDER BY tc.created_at ASC

