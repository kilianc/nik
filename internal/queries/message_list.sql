-- ?1: conversation_id, ?2: before message id (empty to skip), ?3: limit
SELECT
  m.id,
  m.conversation_id,
  m.contact_id,
  m.platform,
  m.external_conversation_id,
  m.external_message_id,
  m.external_sender_id,
  m.sent_at,
  m.is_from_me,
  m.is_group,
  m.kind,
  m.body,
  m.mime_type,
  m.is_edit,
  m.edit_target_message_id,
  m.context_stanza_id,
  m.context_participant,
  m.context_is_forwarded,
  m.context_forwarding_score,
  m.context_mentioned_ids,
  m.is_ephemeral,
  m.is_view_once,
  mm.media_id,
  media.local_path,
  media.describe_text,
  media.transcript_text,
  m.created_at
FROM message m
LEFT JOIN message_media mm ON mm.message_id = m.id
LEFT JOIN media ON media.id = mm.media_id
WHERE m.conversation_id = ?1
  AND (?2 = '' OR m.id < ?2)
ORDER BY m.sent_at DESC, m.id DESC
LIMIT ?3;
