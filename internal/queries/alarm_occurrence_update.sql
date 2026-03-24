UPDATE alarm_occurrence
SET
  note = ?2
WHERE id = (SELECT id FROM alarm_occurrence WHERE alarm_id = ?1 ORDER BY fired_at DESC LIMIT 1)

