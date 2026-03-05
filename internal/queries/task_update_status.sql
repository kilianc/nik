UPDATE task
SET status = ?2,
    completed_at = CASE WHEN ?2 IN ('completed', 'failed', 'cancelled') THEN datetime('now') ELSE completed_at END
WHERE id = ?1
