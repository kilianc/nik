---
name: messaging
preload: true
summary: Send messages, reactions, and typing indicators across platforms.
tools:
  - message_send
  - message_react
---

All tools use canonical nik IDs. Pass empty `conversation_id` for current conversation. Message targeting uses exact content + timestamp from the timeline.

## message_send

Each array item becomes a separate bubble. One thought per message.

- `conversation_id` -- empty = current conversation
- `contact_id` -- set only when starting a new DM
- `messages` -- array of `{text, image_path, voice, quote_text, quote_time}` sent in order
- `quote_text` / `quote_time` -- almost always empty. Only set to anchor to a specific message (see quote reply rules in conversation prompt). Never quote-reply to the message directly above you in a 1:1 DM.

## message_react

React to a message with one emoji.

- `text` -- exact content after sender name, before any `(quote replying to ...)`/`(reacting to ...)`/`(edit of ...)` suffix
- `time` -- HH:MM:SS from timeline brackets
- `emoji` -- reaction emoji

Examples:
- `where? (quote replying to [09:12:45] Bob: ok)` → text="ok", time="09:12:45"
- `(👍) (reacting to [09:12:30] Alice: hello)` → text="hello", time="09:12:30"

## Behavior

- Typing indicators are automatic.
- Reactions are cheap. When acknowledging progress on someone's request, react instead of replying:
  ⏰ alarms, 🔕 cancelling, 👤 contacts, 🔍 researching, 🫡 tasks, 🎛️ config, 👀 media, 🧠 memory.
  One react per message, pick the most relevant. Don't react during autonomous activations.

## WhatsApp Formatting

`*bold*`, `_italic_`, `~strikethrough~`, `` `code` ``, ```` ```block``` ````, `> quote`, `- list` / `* list`, `1. numbered`
