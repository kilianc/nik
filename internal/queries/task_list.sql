SELECT
  id,
  goal,
  status,
  conversation_id,
  created_at,
  started_at,
  completed_at
FROM task
WHERE (?1 = '' OR conversation_id = ?1)
  AND (
    status IN ('pending', 'running')
    OR (?2 AND status IN ('completed', 'failed', 'cancelled') AND completed_at > datetime('now', ?3))
  )
ORDER BY
  CASE WHEN status IN ('pending', 'running') THEN 0 ELSE 1 END,
  created_at DESC
LIMIT 20

