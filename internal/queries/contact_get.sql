-- ?1: identifier (UUID string or WhatsApp JID)
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
WHERE id = ?1
   OR EXISTS (SELECT 1 FROM json_each(whatsapp_ids) WHERE value = ?1)
   OR EXISTS (SELECT 1 FROM json_each(telegram_ids) WHERE value = ?1)
   OR EXISTS (SELECT 1 FROM json_each(slack_ids) WHERE value = ?1)
