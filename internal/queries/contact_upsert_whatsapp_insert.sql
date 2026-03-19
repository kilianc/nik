-- insert a contact only if no existing contact has this WhatsApp JID.
-- ?1: id (UUIDv7 string)  ?2: name  ?3: jid  ?4: phone  ?5: last_message_at
INSERT INTO contact (id, name, nicknames, whatsapp_ids, phone_numbers, last_message_at, created_at, updated_at)
SELECT
  ?1,
  ?2,
  CASE WHEN ?2 = '' THEN '[]' ELSE json_array(?2) END,
  json_array(?3),
  CASE WHEN ?4 = '' THEN '[]' ELSE json_array(?4) END,
  ISO8601_MS(?5),
  NOW_ISO8601_MS(),
  NOW_ISO8601_MS()
WHERE NOT EXISTS (
  SELECT 1 FROM contact WHERE EXISTS (SELECT 1 FROM json_each(whatsapp_ids) WHERE value = ?3)
);
