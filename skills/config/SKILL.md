---
name: config
summary: Read and update nik's runtime config and conversation allow list. Owner-only.
tools: [config]
---

# Config

## config

Read or change nik's runtime configuration.

### Actions

- `get` -- returns the current config (model, timezone, location,
  exa_api_key, media_dir, max_history, allow list, owner)
- `set` -- update a writable field
- `allow_add` -- add a conversation_id to the allow list
- `allow_remove` -- remove a conversation_id from the allow list
- `allow_reload` -- reload the allow list from config.yaml on disk

### Parameters

- `action` -- one of: get, set, allow_add, allow_remove, allow_reload
- `field` -- config field name (for `set`). Writable: `timezone`,
  `location`, `model`, `media_dir`, `max_history`.
- `value` -- new value (for `set`), or conversation_id (for
  allow_add / allow_remove)

### Read-only fields

`privileged_conversation_ids` and `openai_key` cannot be changed via
this tool.

### Allow list

The allow list controls which conversations nik listens to. Only
privileged conversations can modify it. Privileged conversations are
always in the list and cannot be removed.

## Notes

- This tool is **privileged** (owner-only).
- Changes are persisted to `config.yaml` on disk immediately.
