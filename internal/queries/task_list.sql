SELECT
  id,
  source,
  source_id,
  activation_id,
  crew_member_id,
  goal,
  plan,
  thinking,
  status,
  created_at,
  started_at,
  completed_at
FROM task
WHERE source = ?1
  AND source_id = ?2
ORDER BY created_at DESC
