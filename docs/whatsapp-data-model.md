# WhatsApp Data Model (whatsmeow)

Reference of the actual WhatsApp protocol types from `go.mau.fi/whatsmeow` (v0.0.0-20260218135554-9cbe80fb25a4). This documents what data flows through WhatsApp into nik, to inform our generic messaging schema design.

---

## JID (`types.JID`) -- the universal WhatsApp identifier

```go
type JID struct {
    User       string
    RawAgent   uint8
    Device     uint16
    Integrator uint16
    Server     string
}
```

JID servers (the `@suffix` part):

| Server                | Constant             | What it is                              |
|-----------------------|----------------------|-----------------------------------------|
| `s.whatsapp.net`      | `DefaultUserServer`  | Normal users (phone number as prefix)   |
| `g.us`                | `GroupServer`         | Groups                                  |
| `lid`                 | `HiddenUserServer`   | Linked device ID (same person, alt addr)|
| `msgr`                | `MessengerServer`    | Messenger interop                       |
| `interop`             | `InteropServer`      | Cross-platform interop                  |
| `newsletter`          | `NewsletterServer`   | Channels/newsletters                    |
| `broadcast`           | `BroadcastServer`    | Broadcast lists                         |
| `bot`                 | `BotServer`          | WhatsApp bots (Meta AI, etc.)           |
| `c.us`                | `LegacyUserServer`   | Legacy user server                      |
| `hosted`              | `HostedServer`       | Hosted accounts                         |
| `hosted.lid`          | `HostedLIDServer`    | Hosted linked device IDs                |

Stringified examples: `15102953635@s.whatsapp.net`, `120363406922669472@g.us`, `<opaque>@lid`

AD-JIDs (with agent/device): `15102953635.0:1@s.whatsapp.net` (user.agent:device@server)

---

## MessageSource (`types.MessageSource`) -- who sent what where

```go
type MessageSource struct {
    Chat     JID              // which conversation
    Sender   JID              // who sent it
    IsFromMe bool
    IsGroup  bool
    AddressingMode AddressingMode  // "pn" (phone number) or "lid"
    SenderAlt      JID             // alternate address of sender
    RecipientAlt   JID             // alternate address of recipient (DMs)
    BroadcastListOwner  JID
    BroadcastRecipients []BroadcastRecipient
}
```

---

## MessageInfo (`types.MessageInfo`) -- metadata on every message

```go
type MessageInfo struct {
    MessageSource                    // embedded: Chat, Sender, IsFromMe, IsGroup, etc.
    ID        MessageID              // string, e.g. "3EB0..." hex
    ServerID  MessageServerID        // int, for newsletters
    Type      string
    PushName  string                 // sender's WhatsApp display name at time of message
    Timestamp time.Time
    Category  string
    Multicast bool
    MediaType string
    Edit      EditAttribute          // "1"=edit, "7"=sender revoke, "8"=admin revoke
    MsgBotInfo  MsgBotInfo
    MsgMetaInfo MsgMetaInfo
    VerifiedName   *VerifiedName
    DeviceSentMeta *DeviceSentMeta   // for messages sent from another of your own devices
}
```

EditAttribute values:
- `""` -- normal message
- `"1"` -- message edit
- `"2"` -- pin in chat
- `"7"` -- sender revoke (delete for everyone)
- `"8"` -- admin revoke

---

## events.Message -- the top-level event nik receives

```go
type Message struct {
    Info    types.MessageInfo        // all the metadata above
    Message *waE2E.Message           // the protobuf payload (content)

    IsEphemeral           bool       // unwrapped from EphemeralMessage
    IsViewOnce            bool       // unwrapped from ViewOnceMessage
    IsViewOnceV2          bool
    IsViewOnceV2Extension bool
    IsDocumentWithCaption bool
    IsLottieSticker       bool
    IsBotInvoke           bool
    IsEdit                bool       // unwrapped from EditedMessage

    SourceWebMsg *waWeb.WebMessageInfo   // if from history sync
    UnavailableRequestID types.MessageID // if response to unavailable message request
    RetryCount int                       // if re-requested from sender
    NewsletterMeta *NewsletterMessageMeta
    RawMessage *waE2E.Message            // raw unmodified protobuf before unwrapping
}
```

---

## waE2E.Message -- the protobuf content (all possible message types)

Only one field is non-nil per message. This is the full list:

### Core content types

| Field                    | Protobuf Type            | Description                           |
|--------------------------|--------------------------|---------------------------------------|
| `Conversation`           | `*string`                | Plain text message                    |
| `ExtendedTextMessage`    | `*ExtendedTextMessage`   | Text + link preview/formatting        |
| `ImageMessage`           | `*ImageMessage`          | Photo with caption, dimensions        |
| `AudioMessage`           | `*AudioMessage`          | Voice note or audio file              |
| `VideoMessage`           | `*VideoMessage`          | Video with caption, dimensions        |
| `DocumentMessage`        | `*DocumentMessage`       | File attachment (PDF, etc.)           |
| `StickerMessage`         | `*StickerMessage`        | Sticker (static, animated, lottie)    |
| `LocationMessage`        | `*LocationMessage`       | GPS coordinates + name + address      |
| `LiveLocationMessage`    | `*LiveLocationMessage`   | Real-time location sharing            |
| `ContactMessage`         | `*ContactMessage`        | Shared contact vCard                  |
| `ContactsArrayMessage`   | `*ContactsArrayMessage`  | Multiple shared contacts              |

### Interactive / social types

| Field                    | Protobuf Type            | Description                           |
|--------------------------|--------------------------|---------------------------------------|
| `ReactionMessage`        | `*ReactionMessage`       | Emoji reaction to another message     |
| `PollCreationMessage`    | `*PollCreationMessage`   | Create a poll                         |
| `PollUpdateMessage`      | `*PollUpdateMessage`     | Vote on a poll                        |
| `EventMessage`           | `*EventMessage`          | Calendar/event                        |
| `CommentMessage`         | `*CommentMessage`        | Comment on a message                  |
| `AlbumMessage`           | `*AlbumMessage`          | Multi-image album                     |
| `PtvMessage`             | `*VideoMessage`          | Video note (round video, PTV)         |
| `GroupInviteMessage`     | `*GroupInviteMessage`    | Group invite link                     |

### System / protocol types

| Field                    | Protobuf Type            | Description                           |
|--------------------------|--------------------------|---------------------------------------|
| `ProtocolMessage`        | `*ProtocolMessage`       | Delete, ephemeral timer, key rotation |
| `EditedMessage`          | `*FutureProofMessage`    | Message edit wrapper                  |
| `CallLogMesssage`        | `*CallLogMessage`        | Call log entry                        |
| `DeviceSentMessage`      | `*DeviceSentMessage`     | Sent from another device              |
| `EphemeralMessage`       | `*FutureProofMessage`    | Disappearing message wrapper          |
| `ViewOnceMessage`        | `*FutureProofMessage`    | View-once wrapper (v1)                |
| `ViewOnceMessageV2`      | `*FutureProofMessage`    | View-once wrapper (v2)                |

### Business / bot types

| Field                        | Protobuf Type               | Description                       |
|------------------------------|------------------------------|-----------------------------------|
| `TemplateMessage`            | `*TemplateMessage`           | Business template                 |
| `ListMessage`                | `*ListMessage`               | List selector                     |
| `ButtonsMessage`             | `*ButtonsMessage`            | Button options                    |
| `InteractiveMessage`         | `*InteractiveMessage`        | Rich interactive                  |
| `ProductMessage`             | `*ProductMessage`            | Product catalog item              |
| `OrderMessage`               | `*OrderMessage`              | Order                             |
| `BotInvokeMessage`           | `*FutureProofMessage`        | Bot invocation wrapper            |
| `RichResponseMessage`        | `*AIRichResponseMessage`     | AI rich response                  |

---

## Key media structs (storage-relevant fields)

### ImageMessage

```go
type ImageMessage struct {
    URL             *string        // encrypted media download URL
    Mimetype        *string        // e.g. "image/jpeg"
    Caption         *string        // user-provided caption
    FileSHA256      []byte         // hash of decrypted file
    FileLength      *uint64        // file size in bytes
    Height          *uint32
    Width           *uint32
    MediaKey        []byte         // decryption key
    DirectPath      *string        // CDN path
    JPEGThumbnail   []byte         // inline thumbnail
    ViewOnce        *bool
    ContextInfo     *ContextInfo   // reply/quote context
}
```

### AudioMessage

```go
type AudioMessage struct {
    URL             *string
    Mimetype        *string        // e.g. "audio/ogg; codecs=opus"
    FileSHA256      []byte
    FileLength      *uint64
    Seconds         *uint32        // duration
    PTT             *bool          // true = voice note (push-to-talk), false = audio file
    MediaKey        []byte
    DirectPath      *string
    Waveform        []byte         // audio waveform visualization data
    ViewOnce        *bool
    ContextInfo     *ContextInfo
}
```

### VideoMessage

```go
type VideoMessage struct {
    URL             *string
    Mimetype        *string        // e.g. "video/mp4"
    Caption         *string
    FileSHA256      []byte
    FileLength      *uint64
    Seconds         *uint32        // duration
    Height          *uint32
    Width           *uint32
    MediaKey        []byte
    DirectPath      *string
    GifPlayback     *bool          // true = GIF sent as video
    JPEGThumbnail   []byte
    ContextInfo     *ContextInfo
}
```

Also used for `PtvMessage` (round video notes).

### DocumentMessage

```go
type DocumentMessage struct {
    URL             *string
    Mimetype        *string        // e.g. "application/pdf"
    Title           *string        // document title
    FileName        *string        // original filename
    FileSHA256      []byte
    FileLength      *uint64
    PageCount       *uint32        // for PDFs
    MediaKey        []byte
    DirectPath      *string
    Caption         *string
    JPEGThumbnail   []byte
    ContextInfo     *ContextInfo
}
```

### StickerMessage

```go
type StickerMessage struct {
    URL             *string
    Mimetype        *string        // e.g. "image/webp"
    FileSHA256      []byte
    Height          *uint32
    Width           *uint32
    MediaKey        []byte
    DirectPath      *string
    FileLength      *uint64
    IsAnimated      *bool
    IsLottie        *bool          // lottie JSON sticker
    IsAiSticker     *bool          // AI-generated
    IsAvatar        *bool
    ContextInfo     *ContextInfo
}
```

---

## ReactionMessage

```go
type ReactionMessage struct {
    Key               *waCommon.MessageKey  // identifies the message being reacted to
    Text              *string               // the emoji, e.g. "👍" (empty = reaction removed)
    GroupingKey       *string
    SenderTimestampMS *int64
}
```

---

## LocationMessage

```go
type LocationMessage struct {
    DegreesLatitude   *float64
    DegreesLongitude  *float64
    Name              *string       // place name
    Address           *string       // street address
    URL               *string       // maps URL
    IsLive            *bool
    AccuracyInMeters  *uint32
    Comment           *string
    JPEGThumbnail     []byte        // map preview
    ContextInfo       *ContextInfo
}
```

---

## ExtendedTextMessage

```go
type ExtendedTextMessage struct {
    Text             *string              // the actual text
    MatchedText      *string              // URL that was matched for preview
    Description      *string              // link preview description
    Title            *string              // link preview title
    JPEGThumbnail    []byte               // link preview image
    ContextInfo      *ContextInfo         // reply/quote context
    ViewOnce         *bool
}
```

---

## ContextInfo -- reply/quote context (on any message type)

```go
type ContextInfo struct {
    StanzaID        *string              // ID of the quoted/replied-to message
    Participant     *string              // JID of the sender of the quoted message
    QuotedMessage   *Message             // the actual quoted message content
    RemoteJID       *string              // for cross-chat quotes
    MentionedJID    []string             // @mentioned JIDs
    IsForwarded     *bool
    ForwardingScore *uint32              // how many times forwarded
    GroupSubject    *string
    // ... expiration, disappearing mode, ads, etc.
}
```

---

## GroupInfo (`types.GroupInfo`) -- group metadata

```go
type GroupInfo struct {
    JID              JID
    OwnerJID         JID
    GroupName                          // Name, NameSetAt, NameSetBy
    GroupTopic                         // Topic (description), TopicSetAt, TopicSetBy
    GroupLocked                        // IsLocked (only admins edit info?)
    GroupAnnounce                      // IsAnnounce (only admins send messages?)
    GroupEphemeral                     // IsEphemeral, DisappearingTimer
    GroupCreated     time.Time
    Participants     []GroupParticipant
    ParticipantCount int
    MemberAddMode    GroupMemberAddMode // "admin_add" or "all_member_add"
    Suspended        bool
}
```

### GroupParticipant

```go
type GroupParticipant struct {
    JID          JID       // primary JID for messaging
    PhoneNumber  JID       // phone-based JID
    LID          JID       // linked device ID
    IsAdmin      bool
    IsSuperAdmin bool
    DisplayName  string    // for anonymous users in announcement groups
}
```

### GroupName / GroupTopic

```go
type GroupName struct {
    Name      string
    NameSetAt time.Time
    NameSetBy JID
}

type GroupTopic struct {
    Topic        string
    TopicID      string
    TopicSetAt   time.Time
    TopicSetBy   JID
    TopicDeleted bool
}
```

---

## events.GroupInfo -- group change event

```go
type GroupInfo struct {
    JID       types.JID
    Sender    *types.JID              // who made the change
    Timestamp time.Time

    Name      *types.GroupName        // name change (nil if unchanged)
    Topic     *types.GroupTopic       // description change
    Locked    *types.GroupLocked      // admin-only edit toggle
    Announce  *types.GroupAnnounce    // admin-only messaging toggle
    Ephemeral *types.GroupEphemeral   // disappearing messages change

    Join      []types.JID            // users who joined/were added
    Leave     []types.JID            // users who left/were removed
    Promote   []types.JID            // promoted to admin
    Demote    []types.JID            // demoted from admin
}
```

---

## Other relevant events

### Receipt

```go
type Receipt struct {
    types.MessageSource
    MessageIDs []types.MessageID
    Timestamp  time.Time
    Type       types.ReceiptType      // delivered, read, read-self, played, sender, retry
    MessageSender types.JID
}
```

### Presence / ChatPresence

```go
type Presence struct {
    From        types.JID
    Unavailable bool
    LastSeen    time.Time
}

type ChatPresence struct {
    types.MessageSource
    State types.ChatPresence          // composing or paused
    Media types.ChatPresenceMedia     // text or media
}
```

### HistorySync

```go
type HistorySync struct {
    Data *waHistorySync.HistorySync   // blob of historical messages from phone
}
```

---

## What nik currently captures vs. what exists

| WhatsApp type          | Currently handled?  | How                                          |
|------------------------|---------------------|----------------------------------------------|
| Plain text             | Yes                 | `GetConversation()` -> stored as text         |
| Extended text          | Yes                 | `GetExtendedTextMessage()` -> stored as text  |
| Audio                  | Yes                 | Downloaded, transcribed via Whisper           |
| Image                  | Yes                 | Downloaded, saved to disk, described via vision|
| Video                  | No                  | Dropped as `(non-text)`                       |
| Document               | No                  | Dropped                                       |
| Sticker                | No                  | Dropped                                       |
| Location               | No                  | Dropped                                       |
| Contact (vCard)        | No                  | Dropped                                       |
| Reaction               | No                  | Dropped (nik sends reactions but doesn't store incoming ones) |
| Poll                   | No                  | Dropped                                       |
| Edit                   | No                  | Dropped                                       |
| View-once              | No                  | Dropped                                       |
| Group info changes     | Partial             | Join events + name changes stored             |
| Receipts               | No                  | Logged but not stored                         |
| Presence               | No                  | Logged but not stored                         |
