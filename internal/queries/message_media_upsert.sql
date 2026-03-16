-- ?1: id, ?2: message_id, ?3: media_id
INSERT INTO message_media (
  id,
  message_id,
  media_id,
  created_at
)
VALUES (?1, ?2, ?3, datetime('now'))
ON CONFLICT (message_id) DO UPDATE SET
  media_id = excluded.media_id;
