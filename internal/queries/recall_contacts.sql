SELECT
  name,
  nicknames,
  emails,
  phone_numbers,
  whatsapp_ids,
  timezone,
  location,
  one_liner,
  notes
FROM contact
WHERE name != ''
ORDER BY last_message_at DESC NULLS LAST
