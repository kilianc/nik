---
name: config
summary: >
  Load this skill to learn how to read or update nik's runtime
  configuration and manage the conversation allow list.
tools: [update_config]
---

# Config

## update_config

Read or change nik's runtime configuration.

### Actions

- `get` -- returns the current config (model, timezone, location,
  media_dir, debug_dir, max_history, allow list, owner)
- `set` -- update a writable field
- `allow_add` -- add a conversation_id to the allow list
- `allow_remove` -- remove a conversation_id from the allow list
- `allow_reload` -- reload the allow list from config.yaml on disk

### Parameters

- `action` -- one of: get, set, allow_add, allow_remove, allow_reload
- `field` -- config field name (for `set`). Writable: `timezone`,
  `location`, `model`, `debug_dir`, `media_dir`, `max_history`.
- `value` -- new value (for `set`), or conversation_id (for
  allow_add / allow_remove)

### Read-only fields

`owner_conversation_id` and `openai_key` cannot be changed via this
tool.

### Allow list

The allow list controls which conversations nik listens to. Only the
owner can modify it. The owner's conversation is always in the list and
cannot be removed.

## Notes

- This tool is **privileged** (owner-only).
- Changes are persisted to `config.yaml` on disk immediately.
