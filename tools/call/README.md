# call

Invoke any registered brain tool from the command line.

## Usage

```
go run ./tools/call <tool_name> '<json_args>'
```

Examples:

```
go run ./tools/call describe_media '{"file_path":"media/chat/msg.oga","question":""}'
go run ./tools/call message_update_media_description '{"message_id":"3A...","description":"hello","body":""}'
go run ./tools/call shell '{"action":"list","command":"","description":"","session_id":"","input":"","max_wait":0,"next_check_at":"","watch_for":""}'
```

## Available tools

LLM tools (`describe_media`), canonical DB-backed messaging tools (`message_update_media_description`), contact tools, db query tools (`db_query`), shell tools (`shell`), skill tools (`load_skill`), and task tools (`task_spawn`, `task_retry`, `task_list`, `task_status`, `task_cancel`). Live platform actions (`message_reply`, `message_react`, typing/presence) are excluded.

Run with an unknown tool name to see the full list:

```
go run ./tools/call unknown '{}'
```

## Notes

- Loads config from `config.yaml` in CWD (needs `openai_api_key`)
- Falls back gracefully when `nik.db` is locked by a running nik process (LLM tools still work)
