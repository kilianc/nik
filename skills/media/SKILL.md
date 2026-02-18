---
name: media
summary: >
  Load this skill to learn how to describe or transcribe media files
  (images, audio, documents, stickers) and persist the results.
tools: [describe_media]
---

# Media

## describe_media

Describe or transcribe a media file on disk.

- `file_path` -- path to the media file (use the `media=` value from conversation context directly)
- `question` -- optional specific question about the media. If empty, a
  generic description is generated.

### Behavior by type

- **Audio** (.ogg, .oga, .mp3, .m4a, .wav, .flac, .webm): returns a
  transcript of the spoken content.
- **Images** (.jpg, .png, .webp, etc.): returns a visual description,
  or answers the specific question if provided.
- **Documents / stickers**: returns a content description.
- **Video**: not supported.

### Workflow

1. When you see an unprocessed media attachment in conversation
   (shown as `[... attached: /path/...]`), call `describe_media` on it
   first.
2. After getting the description, call `message_update_media_description`
   to persist it so future activations see the result without
   re-processing.
3. Use the `question` parameter to re-examine a past photo when someone
   asks something specific about it.
