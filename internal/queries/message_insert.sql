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
ON CONFLICT (platform, external_message_id) DO NOTHING
