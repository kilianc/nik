SELECT
  ao.id,
  a.id AS alarm_id,
  ao.note,
  ao.fired_at,
  a.goal,
  a.recurrence
FROM alarm_occurrence ao
JOIN alarm a ON a.id = ao.alarm_id
WHERE a.origin_conversation_id = ?1
  AND ao.fired_at >= ?2
  AND a.cancelled_at IS NULL
ORDER BY ao.fired_at
