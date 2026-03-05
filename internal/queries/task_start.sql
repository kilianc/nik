UPDATE task
SET activation_id = ?2,
    status = 'running',
    started_at = datetime('now')
WHERE id = ?1
