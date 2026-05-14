SELECT COUNT(*)
FROM task
WHERE status IN ('pending', 'running')
