-- ?1: local_path
SELECT
  med.id AS media_id,
  m.id AS message_id,
  m.conversation_id,
  m.platform,
  m.external_message_id
FROM media med
JOIN message_media mm ON mm.media_id = med.id
JOIN message m ON m.id = mm.message_id
WHERE med.local_path = ?1;
