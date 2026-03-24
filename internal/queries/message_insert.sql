-- insert or update message by platform external id
-- ?1: id, ?2: conversation_id, ?3: contact_id, ?4: platform, ?5: external_conversation_id
-- ?6: external_message_id, ?7: external_sender_id, ?8: sent_at
-- ?9: is_from_me, ?10: is_group, ?11: kind, ?12: body, ?13: mime_type
-- ?14: is_edit, ?15: edit_target_message_id, ?16: context_stanza_id, ?17: context_participant
-- ?18: context_is_forwarded, ?19: context_forwarding_score, ?20: context_mentioned_ids
-- ?21: is_ephemeral, ?22: is_view_once
INSERT INTO message (
  id,
  conversation_id,
  contact_id,
  platform,
  external_conversation_id,
  external_message_id,
  external_sender_id,
  sent_at,
  is_from_me,
  is_group,
  kind,
  body,
  mime_type,
  is_edit,
  edit_target_message_id,
  context_stanza_id,
  context_participant,
  context_is_forwarded,
  context_forwarding_score,
  context_mentioned_ids,
  is_ephemeral,
  is_view_once,
  created_at
)
VALUES (
  ?1, ?2, ?3, ?4, ?5, ?6, ?7, ISO8601_MS(?8),
  ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17,
  ?18, ?19, ?20, ?21, ?22, NOW_ISO8601_MS()
)
ON CONFLICT (platform, external_message_id) DO UPDATE SET
  contact_id = excluded.contact_id,
  conversation_id = excluded.conversation_id,
  external_conversation_id = excluded.external_conversation_id,
  external_sender_id = excluded.external_sender_id,
  sent_at = excluded.sent_at,
  is_from_me = excluded.is_from_me,
  is_group = excluded.is_group,
  kind = excluded.kind,
  body = excluded.body,
  mime_type = COALESCE(excluded.mime_type, message.mime_type),
  is_edit = excluded.is_edit,
  edit_target_message_id = COALESCE(excluded.edit_target_message_id, message.edit_target_message_id),
  context_stanza_id = COALESCE(excluded.context_stanza_id, message.context_stanza_id),
  context_participant = COALESCE(excluded.context_participant, message.context_participant),
  context_is_forwarded = excluded.context_is_forwarded,
  context_forwarding_score = COALESCE(excluded.context_forwarding_score, message.context_forwarding_score),
  context_mentioned_ids = excluded.context_mentioned_ids,
  is_ephemeral = excluded.is_ephemeral,
  is_view_once = excluded.is_view_once
