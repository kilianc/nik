-- ?1: conversation_id, ?2: contact_id, ?3: display_name
INSERT INTO conversation_participant (
  conversation_id,
  contact_id,
  display_name,
  created_at,
  updated_at
)
VALUES (?1, ?2, ?3, datetime('now'), datetime('now'))
ON CONFLICT (conversation_id, contact_id) DO UPDATE SET
  display_name = COALESCE(excluded.display_name, conversation_participant.display_name),
  updated_at = datetime('now');
