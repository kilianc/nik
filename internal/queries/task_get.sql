SELECT
  id,
  conversation_id,
  contact_id,
  activation_id,
  retry_for_task_id,
  retry_number,
  goal,
  plan,
  thinking,
  status,
  created_at,
  started_at,
  completed_at,
  last_report_at
FROM task
WHERE id = ?1

