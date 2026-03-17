-- update an existing contact matched by WhatsApp JID.
-- ?1: name  ?2: phone  ?3: last_message_at  ?4: jid
UPDATE contact
SET
  name = CASE WHEN name = '' THEN ?1 ELSE name END,
  nicknames = CASE
    WHEN ?1 != '' AND NOT EXISTS (SELECT 1 FROM json_each(nicknames) WHERE value = ?1)
    THEN json_insert(nicknames, '$[#]', ?1)
    ELSE nicknames
  END,
  phone_numbers = CASE
    WHEN ?2 != '' AND NOT EXISTS (SELECT 1 FROM json_each(phone_numbers) WHERE value = ?2)
    THEN json_insert(phone_numbers, '$[#]', ?2)
    ELSE phone_numbers
  END,
  last_message_at = CASE
    WHEN last_message_at IS NULL OR ISO8601_MS(?3) > last_message_at THEN ISO8601_MS(?3)
    ELSE last_message_at
  END,
  updated_at = NOW_ISO8601_MS()
WHERE EXISTS (SELECT 1 FROM json_each(whatsapp_ids) WHERE value = ?4);
