-- ?1: id, ?2: status, ?3: activation_id, ?4: last_report_at, ?5: cancellation_reason, ?6: plan
UPDATE task
SET
  status = COALESCE(?2, status),
  activation_id = COALESCE(?3, activation_id),
  started_at = CASE
    WHEN ?2 = 'running' THEN NOW_ISO8601_MS()
    ELSE started_at
  END,
  completed_at = CASE
    WHEN ?2 IN ('completed', 'failed', 'cancelled') THEN NOW_ISO8601_MS()
    ELSE completed_at
  END,
  last_report_at = COALESCE(NULLABLE_ISO8601_MS(?4), last_report_at),
  cancellation_reason = COALESCE(?5, cancellation_reason),
  plan = COALESCE(?6, plan)
WHERE id = ?1
