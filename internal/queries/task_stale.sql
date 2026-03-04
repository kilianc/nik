SELECT
  t.id,
  t.source,
  t.source_id,
  t.activation_id,
  t.goal,
  t.plan,
  t.thinking,
  t.status,
  t.created_at,
  t.started_at,
  t.completed_at
FROM task t
WHERE t.status = 'running'
  AND t.activation_id IS NOT NULL
  AND (
    (SELECT MAX(tc.created_at) FROM tool_call tc WHERE tc.activation_id = t.activation_id) < ?1
    OR (
      NOT EXISTS (SELECT 1 FROM tool_call tc WHERE tc.activation_id = t.activation_id)
      AND t.started_at < ?1
    )
  )
