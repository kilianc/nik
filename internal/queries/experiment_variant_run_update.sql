UPDATE experiment_variant_run
SET
  is_desired = ?2,
  rationale = ?3
WHERE id = ?1
