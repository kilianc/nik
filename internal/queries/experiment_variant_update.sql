UPDATE experiment_variant
SET
  status = COALESCE(?2, status),
  run_count = COALESCE(?3, run_count),
  desired_count = COALESCE(?4, desired_count),
  updated_at = NOW_ISO8601_MS()
WHERE id = ?1
