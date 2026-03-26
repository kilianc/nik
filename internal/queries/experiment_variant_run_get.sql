SELECT
  r.id,
  r.experiment_variant_id,
  r.tool_calls,
  r.model_output,
  r.reasoning_summaries,
  r.is_desired,
  r.rationale,
  r.input_tokens,
  r.output_tokens,
  r.cached_tokens,
  r.reasoning_tokens,
  r.created_at,
  a.model,
  a.instructions,
  a.tool_schemas,
  ar.user_input,
  COALESCE(NULLIF(ev.reasoning_effort, ''), a.reasoning_effort),
  COALESCE(NULLIF(ev.verbosity, ''), a.verbosity),
  ev.patches,
  a.id,
  ar.round
FROM experiment_variant_run r
JOIN experiment_variant ev ON ev.id = r.experiment_variant_id
JOIN experiment e ON e.id = ev.experiment_id
JOIN activation_round ar ON ar.id = e.activation_round_id
JOIN activation a ON a.id = ar.activation_id
WHERE r.id = ?1
