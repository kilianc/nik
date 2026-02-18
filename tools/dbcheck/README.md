# dbcheck

Opens `nik.db` read-only and runs integrity and summary checks.

## Usage

```
make db-check
```

## Checks

| Check | What it catches |
|-------|----------------|
| Row counts | Summary of contacts, conversations, messages, and media |
| Orphan messages | Messages referencing a `conversation_id` not in `conversation` |
| Orphan contact refs | Messages with a `contact_id` not in `contact` |
| Orphan participants | `conversation_participant` rows pointing to missing conversation/contact |
| Orphan message media | `message_media` links pointing to missing `message` or `media` |
| Stale conversation timestamps | `conversation.last_message_at` older than latest `message.sent_at` |
| Duplicate contact JIDs | Same WhatsApp JID appearing in multiple contacts |
| Duplicate external IDs | Duplicate (`platform`, `external_message_id`) rows |
| Empty text messages | Messages with `kind=text` and empty body |
| Message kinds | Breakdown by kind (text, image, audio, etc.) |
| Time range | Earliest and latest message timestamps |

Exits with code 1 if any issues are found.
