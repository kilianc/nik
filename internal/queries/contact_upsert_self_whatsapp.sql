-- ?1: self contact id, ?2: whatsapp jid, ?3: last_message_at
INSERT INTO contact (
  id,
  name,
  nicknames,
  whatsapp_ids,
  last_message_at,
  created_at,
  updated_at
)
VALUES (
  ?1,
  'nik',
  json_array('nik'),
  json_array(?2),
  ?3,
  datetime('now'),
  datetime('now')
)
ON CONFLICT (id) DO UPDATE SET
  name = CASE WHEN contact.name = '' THEN 'nik' ELSE contact.name END,
  nicknames = CASE
    WHEN NOT EXISTS (SELECT 1 FROM json_each(contact.nicknames) WHERE value = 'nik')
    THEN json_insert(contact.nicknames, '$[#]', 'nik')
    ELSE contact.nicknames
  END,
  whatsapp_ids = CASE
    WHEN NOT EXISTS (SELECT 1 FROM json_each(contact.whatsapp_ids) WHERE value = ?2)
    THEN json_insert(contact.whatsapp_ids, '$[#]', ?2)
    ELSE contact.whatsapp_ids
  END,
  last_message_at = CASE
    WHEN contact.last_message_at IS NULL OR ?3 > contact.last_message_at THEN ?3
    ELSE contact.last_message_at
  END,
  updated_at = datetime('now');
