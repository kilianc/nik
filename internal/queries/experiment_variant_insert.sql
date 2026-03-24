INSERT INTO experiment_variant (
  id,
  experiment_id,
  name,
  status,
  hypothesis,
  patches,
  reasoning_effort,
  verbosity
)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)
