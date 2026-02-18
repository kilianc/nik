-- ?1: conversation id (empty to skip), ?2: platform, ?3: external_conversation_id
SELECT
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
  last_read_at,
  created_at,
  updated_at
FROM conversation
WHERE (?1 != '' AND id = ?1)
   OR (?1 = '' AND platform = ?2 AND external_conversation_id = ?3);
