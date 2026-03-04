INSERT INTO task (
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
)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)
