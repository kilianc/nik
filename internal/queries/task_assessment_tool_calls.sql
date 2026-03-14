SELECT
  tc.name,
  tc.input,
  tc.output,
  tc.duration_ms,
  tc.error,
  tc.created_at
FROM tool_call tc
WHERE tc.activation_id = ?1
ORDER BY tc.created_at ASC
