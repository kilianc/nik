INSERT OR IGNORE INTO message (
  id,
  conversation_id,
  contact_id,
  platform,
  external_conversation_id,
  external_message_id,
  external_sender_id,
  kind,
  body,
  sent_at
)
VALUES (
  ?1, ?2, ?3, 'system', ?2, ?4, ?3,
  'text', ?5, NOW_ISO8601_MS()
)
