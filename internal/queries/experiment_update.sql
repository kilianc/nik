UPDATE experiment
SET
  status = COALESCE(?2, status),
  desired_outcome = COALESCE(?3, desired_outcome),
  analysis = COALESCE(?4, analysis),
  updated_at = NOW_ISO8601_MS()
WHERE id = ?1

