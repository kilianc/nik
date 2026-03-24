-- ?1: id (UUID string), ?2: platform, ?3: external_conversation_id, ?4: kind, ?5: title, ?6: last_message_at
-- ?7: topic, ?8: is_announce, ?9: is_locked, ?10: owner_external_id, ?11: participant_external_ids
INSERT INTO conversation (
  id,
  platform,
  external_conversation_id,
  kind,
  title,
  topic,
  is_announce,
  is_locked,
  owner_external_id,
  participant_external_ids,
  last_message_at,
  created_at,
  updated_at
)
VALUES (?1, ?2, ?3, ?4, ?5, ?7, COALESCE(?8, 0), COALESCE(?9, 0), ?10, COALESCE(?11, '[]'), NULLABLE_ISO8601_MS(?6), NOW_ISO8601_MS(), NOW_ISO8601_MS())
ON CONFLICT (platform, external_conversation_id) DO UPDATE SET
  kind = excluded.kind,
  title = COALESCE(excluded.title, conversation.title),
  topic = COALESCE(excluded.topic, conversation.topic),
  is_announce = COALESCE(?8, conversation.is_announce),
  is_locked = COALESCE(?9, conversation.is_locked),
  owner_external_id = COALESCE(excluded.owner_external_id, conversation.owner_external_id),
  participant_external_ids = COALESCE(?11, conversation.participant_external_ids),
  last_message_at = CASE
    WHEN excluded.last_message_at IS NULL THEN conversation.last_message_at
    WHEN conversation.last_message_at IS NULL THEN excluded.last_message_at
    WHEN excluded.last_message_at > conversation.last_message_at THEN excluded.last_message_at
    ELSE conversation.last_message_at
  END,
  updated_at = NOW_ISO8601_MS()
