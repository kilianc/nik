UPDATE activation
SET
  instructions = ?2,
  tools = ?3
WHERE id = ?1
