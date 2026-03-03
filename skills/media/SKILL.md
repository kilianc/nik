---
name: media
summary: Describe or transcribe images, audio, stickers, and documents attached to messages.
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
   (identified by `media=` like `(audio) | media=media/abc.ogg`),
   call `describe_media` with the `media=` value as `file_path`.
2. After getting the description, call `message_update_media_description`
   to persist it so future activations see the result without
   re-processing.
3. If a message shows `media_unavailable` instead of a `media=` path,
   the file was not downloaded. Do not call `describe_media` --
   acknowledge you can't process it.
4. Use the `question` parameter to re-examine a past photo when someone
   asks something specific about it.
