SELECT
  t.id AS task_id,
  t.goal,
  t.status,
  t.meta,
  t.retry_for_task_id,
  t.retry_number,
  COALESCE(GROUP_CONCAT(tr.id), '') AS report_ids,
  COALESCE(GROUP_CONCAT(tr.content, char(10) || char(10)), '') AS reports
FROM task t
LEFT JOIN task_report tr ON tr.task_id = t.id AND tr.reported_at IS NULL
WHERE t.status != 'cancelled'
  AND (
    tr.id IS NOT NULL
    OR (
      t.status IN ('completed', 'failed')
      AND (t.checked_at IS NULL OR t.completed_at > t.checked_at)
    )
  )
GROUP BY t.id
ORDER BY COALESCE(MIN(tr.created_at), t.completed_at) ASC
