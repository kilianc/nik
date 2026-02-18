-- ?1: contact id, ?2: whatsapp jid to add
UPDATE contact
SET whatsapp_ids = json_insert(whatsapp_ids, '$[#]', ?2),
    phone_numbers = CASE
      WHEN ?3 != '' AND NOT EXISTS (SELECT 1 FROM json_each(phone_numbers) WHERE value = ?3)
      THEN json_insert(phone_numbers, '$[#]', ?3)
      ELSE phone_numbers
    END,
    updated_at = datetime('now')
WHERE id = ?1
  AND NOT EXISTS (SELECT 1 FROM json_each(whatsapp_ids) WHERE value = ?2);
