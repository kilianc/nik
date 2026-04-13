# wapp-history-dump

Pairs with WhatsApp via QR code, records all HistorySync protobuf chunks to a file, and exits after 30 seconds of inactivity.

## Usage

```
make wapp-history-dump
```

Or with a custom output path:

```
make wapp-history-dump ARGS=custom_output.pb64
```

Default output file is `wapp_history.pb64`.

## How it works

1. Creates a temporary session DB (cleaned up on exit)
2. Displays a QR code for pairing
3. Records each HistorySync chunk as a base64-encoded protobuf line (NDJSON-style)
4. Exits automatically after 30s with no new chunks, or on Ctrl-C

## Replay

The output file can be replayed through nik's full message processing pipeline:

```
make run-replay ARGS=wapp_history.pb64
```

This runs the `replay` subcommand against a clean DB, useful for debugging ingestion logic without re-triggering a WhatsApp sync.
