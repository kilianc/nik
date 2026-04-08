-- ?1: id, ?2: external_conversation_id, ?3: title, ?4: last_message_at
UPDATE conversation SET
  external_conversation_id = COALESCE(?2, external_conversation_id),
  title = COALESCE(?3, title),
  last_message_at = CASE
    WHEN ?4 IS NULL THEN last_message_at
    WHEN last_message_at IS NULL THEN NULLABLE_ISO8601_MS(?4)
    WHEN NULLABLE_ISO8601_MS(?4) > last_message_at THEN NULLABLE_ISO8601_MS(?4)
    ELSE last_message_at
  END,
  updated_at = NOW_ISO8601_MS()
WHERE id = ?1
