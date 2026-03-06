SELECT
  id,
  meta,
  activation_id,
  crew_member_id,
  retry_for_task_id,
  retry_number,
  goal,
  plan,
  thinking,
  status,
  created_at,
  started_at,
  completed_at
FROM task
WHERE id = ?1
