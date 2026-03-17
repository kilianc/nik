-- ?1: id, ?2: status
UPDATE task
SET status = ?2,
    completed_at = CASE WHEN ?2 IN ('completed', 'failed', 'cancelled') THEN NOW_ISO8601_MS() ELSE completed_at END
WHERE id = ?1
