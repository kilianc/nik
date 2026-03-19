-- ?1: id, ?2: goal, ?3: recurrence, ?4: next_fire_at, ?5: last_fired_at
-- ?6: apply_last_occurrence_note, ?7: last_occurrence_note, ?8: cancel
UPDATE alarm
SET
  goal = COALESCE(?2, goal),
  recurrence = COALESCE(?3, recurrence),
  next_fire_at = COALESCE(NULLABLE_ISO8601_MS(?4), next_fire_at),
  last_fired_at = COALESCE(NULLABLE_ISO8601_MS(?5), last_fired_at),
  last_occurrence_note = CASE
    WHEN ?6 THEN ?7
    ELSE last_occurrence_note
  END,
  cancelled_at = CASE
    WHEN ?8 THEN NOW_ISO8601_MS()
    ELSE cancelled_at
  END
WHERE id = ?1
