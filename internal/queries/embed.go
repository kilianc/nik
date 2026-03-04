package queries

import _ "embed"

// contact queries

//go:embed contact_upsert_whatsapp_insert.sql
var ContactUpsertWhatsAppInsert string

//go:embed contact_upsert_whatsapp_update.sql
var ContactUpsertWhatsAppUpdate string

//go:embed contact_upsert_self_whatsapp.sql
var ContactUpsertSelfWhatsApp string

//go:embed get_contact.sql
var GetContact string

//go:embed list_contacts.sql
var ListContacts string

//go:embed search_contact.sql
var SearchContact string

//go:embed update_contact_field.sql
var UpdateContactField string

//go:embed contact_add_whatsapp_id.sql
var ContactAddWhatsAppID string

// conversation queries

//go:embed conversation_upsert.sql
var ConversationUpsert string

//go:embed conversation_get.sql
var ConversationGet string

//go:embed conversation_poll_unread.sql
var ConversationPollUnread string

//go:embed conversation_mark_read.sql
var ConversationMarkRead string

//go:embed conversation_mark_all_read.sql
var ConversationMarkAllRead string

//go:embed conversation_upsert_participant.sql
var ConversationUpsertParticipant string

//go:embed conversation_get_participants.sql
var ConversationGetParticipants string

// message queries

//go:embed message_insert.sql
var MessageInsert string

//go:embed message_get.sql
var MessageGet string

//go:embed message_list.sql
var MessageList string

//go:embed message_get_around.sql
var MessageGetAround string

//go:embed message_update_body.sql
var MessageUpdateBody string

// media queries

//go:embed media_upsert.sql
var MediaUpsert string

//go:embed media_update_description.sql
var MediaUpdateDescription string

//go:embed message_media_upsert.sql
var MessageMediaUpsert string

// alarm queries

//go:embed create_alarm.sql
var CreateAlarm string

//go:embed due_alarms.sql
var DueAlarms string

//go:embed alarm_claim.sql
var AlarmClaim string

//go:embed alarm_set_next_fire.sql
var AlarmSetNextFire string

//go:embed alarm_update.sql
var AlarmUpdate string

//go:embed alarm_cancel.sql
var AlarmCancel string

// alarm occurrence queries

//go:embed alarm_occurrence_insert.sql
var AlarmOccurrenceInsert string

//go:embed alarm_occurrence_update_note.sql
var AlarmOccurrenceUpdateNote string

//go:embed alarm_occurrence_summary.sql
var AlarmOccurrenceSummary string

// memory queries

//go:embed memory_insert.sql
var MemoryInsert string

//go:embed memory_vec_insert.sql
var MemoryVecInsert string

//go:embed memory_search.sql
var MemorySearch string

//go:embed memory_delete.sql
var MemoryDelete string

//go:embed memory_list.sql
var MemoryList string

//go:embed memory_random.sql
var MemoryRandom string

// journal queries

//go:embed journal_check.sql
var JournalCheck string

//go:embed journal_start.sql
var JournalStart string

//go:embed journal_write.sql
var JournalWrite string

//go:embed journal_conversations_today.sql
var JournalConversationsToday string

//go:embed journal_messages_today.sql
var JournalMessagesToday string

//go:embed journal_contacts_today.sql
var JournalContactsToday string

//go:embed journal_memories_today.sql
var JournalMemoriesToday string

//go:embed journal_get.sql
var JournalGet string

//go:embed journal_crew_today.sql
var JournalCrewToday string

// dream queries

//go:embed dream_check.sql
var DreamCheck string

//go:embed dream_start.sql
var DreamStart string

//go:embed dream_write.sql
var DreamWrite string

//go:embed dream_passes.sql
var DreamPasses string

// briefing queries

//go:embed briefing_check.sql
var BriefingCheck string

//go:embed briefing_start.sql
var BriefingStart string

//go:embed briefing_write.sql
var BriefingWrite string

//go:embed briefing_get.sql
var BriefingGet string

//go:embed briefing_topic_list.sql
var BriefingTopicList string

//go:embed briefing_topic_insert.sql
var BriefingTopicInsert string

//go:embed briefing_topic_delete.sql
var BriefingTopicDelete string

// soul queries

//go:embed soul_current.sql
var SoulCurrent string

//go:embed soul_insert.sql
var SoulInsert string

// activation queries

//go:embed activation_insert.sql
var ActivationInsert string

//go:embed activation_update_stats.sql
var ActivationUpdateStats string

// tool call queries

//go:embed tool_call_insert.sql
var ToolCallInsert string

//go:embed tool_call_insert_one.sql
var ToolCallInsertOne string

// crew member queries

//go:embed crew_member_insert.sql
var CrewMemberInsert string

//go:embed crew_member_list.sql
var CrewMemberList string

//go:embed crew_member_get.sql
var CrewMemberGet string

// task queries

//go:embed task_insert.sql
var TaskInsert string

//go:embed task_get.sql
var TaskGet string

//go:embed task_update_status.sql
var TaskUpdateStatus string

//go:embed task_set_activation_id.sql
var TaskSetActivationID string

//go:embed task_list.sql
var TaskList string

//go:embed task_active.sql
var TaskActive string

//go:embed task_stale.sql
var TaskStale string

//go:embed task_mark_checked.sql
var TaskMarkChecked string

// task report queries

//go:embed task_report_insert.sql
var TaskReportInsert string

//go:embed task_report_unreported.sql
var TaskReportUnreported string

//go:embed task_report_mark_reported.sql
var TaskReportMarkReported string

//go:embed task_tool_calls.sql
var TaskToolCalls string
