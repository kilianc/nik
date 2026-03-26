UPDATE experiment_variant
SET
  run_count = (SELECT COUNT(*) FROM experiment_variant_run WHERE experiment_variant_id = ?1),
  desired_count = (SELECT COUNT(*) FROM experiment_variant_run WHERE experiment_variant_id = ?1 AND is_desired = 1),
  updated_at = NOW_ISO8601_MS()
WHERE id = ?1;
