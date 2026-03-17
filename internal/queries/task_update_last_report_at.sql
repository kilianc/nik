UPDATE task
SET last_report_at = ISO8601_MS(?2)
WHERE id = ?1
