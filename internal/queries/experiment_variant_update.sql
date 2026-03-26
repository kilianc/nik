UPDATE experiment_variant
SET
  run_count = COALESCE(?2, run_count),
  desired_count = COALESCE(?3, desired_count),
  updated_at = NOW_ISO8601_MS()
WHERE id = ?1
