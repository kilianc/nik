-- ?1: message id (UUID string), ?2: body
UPDATE message
SET
  body = ?2
WHERE id = ?1
