UPDATE task
SET status = ?2,
    started_at = CASE WHEN ?2 = 'running' AND started_at IS NULL THEN datetime('now') ELSE started_at END,
    completed_at = CASE WHEN ?2 IN ('completed', 'failed', 'cancelled') THEN datetime('now') ELSE completed_at END
WHERE id = ?1
