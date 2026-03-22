INSERT INTO alarm (
  id,
  origin_contact_id,
  origin_conversation_id,
  goal,
  recurrence,
  next_fire_at,
  created_at
)
VALUES (?1, ?2, ?3, ?4, ?5, ISO8601_MS(?6), ISO8601_MS(?7))
