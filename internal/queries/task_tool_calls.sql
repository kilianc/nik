SELECT
  tc.name,
  tc.duration_ms,
  tc.error,
  tc.created_at
FROM tool_call tc
WHERE tc.activation_id = ?1
ORDER BY tc.created_at DESC
LIMIT 10
