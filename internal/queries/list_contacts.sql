-- ?1: limit  ?2: offset
SELECT
  id,
  name,
  nicknames,
  emails,
  whatsapp_ids,
  telegram_ids,
  slack_ids,
  phone_numbers,
  timezone,
  location,
  one_liner,
  notes,
  last_message_at,
  last_seen_at,
  created_at,
  updated_at
FROM contact
ORDER BY updated_at DESC
LIMIT ?1 OFFSET ?2;
