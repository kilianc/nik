-- ?1: ISO8601 cutoff timestamp — rows with created_at < ?1 are removed.
-- statements must be executed in order within a single transaction.

-- phase 1: activation dependents

DELETE FROM experiment_variant_run
WHERE experiment_variant_id IN (
  SELECT ev.id
  FROM experiment_variant ev
  JOIN experiment e ON ev.experiment_id = e.id
  JOIN activation_round ar ON e.activation_round_id = ar.id
  JOIN activation a ON ar.activation_id = a.id
  WHERE a.created_at < ?1
);

DELETE FROM experiment_variant
WHERE experiment_id IN (
  SELECT e.id
  FROM experiment e
  JOIN activation_round ar ON e.activation_round_id = ar.id
  JOIN activation a ON ar.activation_id = a.id
  WHERE a.created_at < ?1
);

DELETE FROM experiment
WHERE activation_round_id IN (
  SELECT ar.id
  FROM activation_round ar
  JOIN activation a ON ar.activation_id = a.id
  WHERE a.created_at < ?1
);

DELETE FROM tool_call
WHERE activation_id IN (
  SELECT id FROM activation WHERE created_at < ?1
);

DELETE FROM activation_round
WHERE activation_id IN (
  SELECT id FROM activation WHERE created_at < ?1
);

DELETE FROM shell_session
WHERE activation_id IN (
  SELECT id FROM activation WHERE created_at < ?1
);

-- phase 2: detach cross-references

UPDATE task
SET activation_id = NULL
WHERE activation_id IN (
  SELECT id FROM activation WHERE created_at < ?1
);

UPDATE activation
SET task_id = NULL
WHERE task_id IN (
  SELECT id FROM task WHERE created_at < ?1
);

-- phase 3: task dependents + tasks

DELETE FROM task_report
WHERE task_id IN (
  SELECT id FROM task WHERE created_at < ?1
);

UPDATE task
SET retry_for_task_id = NULL
WHERE retry_for_task_id IN (
  SELECT id FROM task WHERE created_at < ?1
);

DELETE FROM task
WHERE created_at < ?1;

-- phase 4: activations

DELETE FROM activation
WHERE created_at < ?1;

-- phase 5: system messages

DELETE FROM message
WHERE platform = 'system'
  AND created_at < ?1;
