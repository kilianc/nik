SELECT
  id,
  alarm_id,
  note,
  next_fire_at_set,
  fired_at
FROM alarm_occurrence
WHERE alarm_id = ?1
ORDER BY fired_at DESC
LIMIT ?2
