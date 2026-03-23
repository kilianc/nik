UPDATE activation
SET
  instructions = ?2,
  tools = ?3,
  tool_schemas = ?4
WHERE id = ?1
