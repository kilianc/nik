SELECT
  id,
  activation_round_id,
  status,
  desired_outcome,
  analysis,
  created_at,
  updated_at
FROM experiment
WHERE id LIKE '%' || ?1

