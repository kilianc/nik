package db

import (
	"database/sql"
	"time"
)

type Contact struct {
	ID            string
	Name          string
	Nicknames     []string
	Emails        []string
	WhatsappIDs   []string
	TelegramIDs   []string
	SlackIDs      []string
	PhoneNumbers  []string
	Timezone      sql.NullString
	Location      sql.NullString
	OneLiner      sql.NullString
	Notes         sql.NullString
	LastMessageAt sql.NullTime
	LastSeenAt    sql.NullTime
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Conversation struct {
	ID                     string
	Platform               string
	ExternalConversationID string
	Kind                   string
	Title                  sql.NullString
	Topic                  sql.NullString
	IsAnnounce             bool
	IsLocked               bool
	OwnerExternalID        sql.NullString
	ParticipantExternalIDs []string
	LastMessageAt          sql.NullTime
	LastReadAt             sql.NullTime
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type ConversationParticipant struct {
	ContactID   string
	DisplayName sql.NullString
	ContactName sql.NullString
	Timezone    sql.NullString
	Location    sql.NullString
	OneLiner    sql.NullString
}

type Message struct {
	ID                     string
	ConversationID         string
	ContactID              string
	Platform               string
	ExternalConversationID string
	ExternalMessageID      string
	ExternalSenderID       string
	SentAt                 time.Time
	IsFromMe               bool
	IsGroup                bool
	Kind                   string
	Body                   string
	MimeType               sql.NullString
	IsEdit                 bool
	EditTargetMessageID    sql.NullString
	ContextStanzaID        sql.NullString
	ContextParticipant     sql.NullString
	ContextIsForwarded     bool
	ContextForwardingScore sql.NullInt32
	ContextMentionedIDs    []string
	IsEphemeral            bool
	IsViewOnce             bool
	MediaID                sql.NullString
	MediaLocalPath         sql.NullString
	MediaDescribeText      sql.NullString
	MediaTranscriptText    sql.NullString
	CreatedAt              time.Time
}

type Media struct {
	ID             string
	MimeType       string
	SizeBytes      sql.NullInt64
	LocalPath      sql.NullString
	DescribeText   sql.NullString
	TranscriptText sql.NullString
	DescribedAt    sql.NullTime
	TranscribedAt  sql.NullTime
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Alarm struct {
	ID                   string
	OriginContactID      sql.NullString
	OriginConversationID sql.NullString
	Goal                 string
	Recurrence           sql.NullString
	Source               sql.NullString
	SourceID             sql.NullString
	NextFireAt           sql.NullTime
	LastFiredAt          sql.NullTime
	CreatedAt            time.Time
}

type AlarmOccurrence struct {
	ID            string
	AlarmID       string
	Note          sql.NullString
	NextFireAtSet sql.NullTime
	FiredAt       time.Time
}

type Task struct {
	ID             string
	Meta           map[string]string
	ActivationID   string
	CrewMemberID   string
	RetryForTaskID string
	RetryNumber    int
	Goal           string
	Plan           string
	Thinking       string
	Status         string
	CreatedAt      time.Time
	StartedAt      sql.NullTime
	CompletedAt    sql.NullTime
}

type ActiveTask struct {
	ID             string
	Goal           string
	Status         string
	ConversationID string
	RetryNumber    int
	CreatedAt      time.Time
}

type TaskListRow struct {
	ID             string
	Goal           string
	Status         string
	ConversationID sql.NullString
	CreatedAt      time.Time
	StartedAt      sql.NullTime
	CompletedAt    sql.NullTime
}

type RetryChainEntry struct {
	ID          string
	RetryNumber int
	Goal        string
	Status      string
	Reports     string
}

type TaskAttention struct {
	TaskID         string
	Goal           string
	Status         string
	Meta           map[string]string
	RetryForTaskID string
	RetryNumber    int
	ReportIDs      string
	Reports        string
}

type ToolCallInfo struct {
	Name       string
	DurationMS int64
	Error      bool
	At         time.Time
}

type Memory struct {
	ID        string
	Content   string
	Metadata  map[string]any
	Source    string
	SourceID  string
	CreatedAt time.Time
	Score     float64
}

type RandomMemory struct {
	ID        string
	Content   string
	CreatedAt time.Time
}

type Soul struct {
	Version int
	Content string
}

type ContactSearchResult struct {
	Contact
	Score float64
}

type BriefingTopic struct {
	ID        string
	Query     string
	Reason    string
	ContactID sql.NullString
	CreatedAt time.Time
}

type DreamPass struct {
	Pass        int
	Content     string
	CompletedAt time.Time
}

type JournalConversation struct {
	ID           string
	Platform     string
	Kind         string
	Title        sql.NullString
	MessageCount int
}

type JournalContact struct {
	ID        string
	Name      string
	Nicknames []string
	OneLiner  sql.NullString
	CreatedAt time.Time
}

type JournalCrewHire struct {
	ID        string
	Name      string
	Prompt    string
	CreatedAt time.Time
	TaskCount int
}

type JournalMemory struct {
	ID        string
	Content   string
	CreatedAt time.Time
}
