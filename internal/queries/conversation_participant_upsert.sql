-- ?1: id, ?2: conversation_id, ?3: contact_id, ?4: display_name
INSERT INTO conversation_participant (
  id,
  conversation_id,
  contact_id,
  display_name,
  created_at,
  updated_at
)
VALUES (?1, ?2, ?3, ?4, NOW_ISO8601_MS(), NOW_ISO8601_MS())
ON CONFLICT (conversation_id, contact_id) DO UPDATE SET
  display_name = COALESCE(excluded.display_name, conversation_participant.display_name),
  updated_at = NOW_ISO8601_MS()
