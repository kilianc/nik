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
WHERE id = ?1
