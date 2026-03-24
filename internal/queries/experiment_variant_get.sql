SELECT
  id,
  experiment_id,
  name,
  status,
  hypothesis,
  patches,
  reasoning_effort,
  verbosity,
  run_count,
  desired_count,
  created_at,
  updated_at
FROM experiment_variant
WHERE id LIKE '%' || ?1
