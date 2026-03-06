SELECT
  t.id,
  t.retry_number,
  t.goal,
  t.status,
  COALESCE(
    GROUP_CONCAT(tr.content, char(10)),
    ''
  ) AS reports
FROM task t
LEFT JOIN task_report tr ON tr.task_id = t.id
WHERE t.retry_for_task_id = ?1 OR t.id = ?1
GROUP BY t.id
ORDER BY t.retry_number
