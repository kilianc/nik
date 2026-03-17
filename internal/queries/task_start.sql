-- ?1: id, ?2: activation_id
UPDATE task
SET activation_id = ?2,
    status = 'running',
    started_at = NOW_ISO8601_MS()
WHERE id = ?1
