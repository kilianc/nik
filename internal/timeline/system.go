package timeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/kciuffolo/nik/internal/alarms"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/id"
)

const (
	pad               = "           " // 11 spaces — width of [HH:MM:SS] + space
	reportTruncateLen = 200
)

func renderSystemMessage(msg db.Message) entry {
	switch msg.Kind {
	case "task_report":
		return renderTaskReport(msg)
	case "task_spawned":
		return renderTaskSpawned(msg)
	case "task_retry":
		return renderTaskRetry(msg)
	case "task_cancelled":
		return renderTaskCancelled(msg)
	case "alarm_fired":
		return renderAlarmFired(msg)
	case "alarm_stale":
		return renderAlarmStale(msg)
	case "alarm_created":
		return renderAlarmCreated(msg)
	case "alarm_updated":
		return renderAlarmUpdated(msg)
	case "skill_added", "skill_removed", "skill_changed":
		return renderSkillEvent(msg)
	case "tool_call":
		return renderToolCall(msg)
	case "trigger":
		return renderTrigger(msg)
	case "skill_reflex_fired":
		return renderSkillReflexFired(msg)
	default:
		return entry{at: msg.SentAt, from: "system", text: "[" + msg.Kind + "] " + msg.Body}
	}
}

func renderTaskReport(msg db.Message) entry {
	var r db.TaskReport
	_ = json.Unmarshal([]byte(msg.Body), &r)

	content := r.Content
	if len(content) > reportTruncateLen {
		content = content[:reportTruncateLen] + " [truncated]"
	}

	lines := []string{
		"[task report]",
		"task_id: " + id.Shorten(r.TaskID),
		"goal: " + r.Goal,
		"status: " + r.Status,
		"report: " + content,
	}

	return entry{at: msg.SentAt, from: "system", text: padLines(lines)}
}

func renderTaskSpawned(msg db.Message) entry {
	var t db.Task
	_ = json.Unmarshal([]byte(msg.Body), &t)

	lines := []string{
		"[task spawned]",
		"task_id: " + id.Shorten(t.ID),
		"goal: " + t.Goal,
	}

	return entry{at: msg.SentAt, from: "system", text: padLines(lines)}
}

func renderTaskRetry(msg db.Message) entry {
	var t db.Task
	_ = json.Unmarshal([]byte(msg.Body), &t)

	lines := []string{
		"[task retry #" + strconv.Itoa(t.RetryNumber) + " spawned]",
		"task_id: " + id.Shorten(t.ID),
		"retry_of: " + id.Shorten(t.RetryForTaskID),
		"goal: " + t.Goal,
	}

	return entry{at: msg.SentAt, from: "system", text: padLines(lines)}
}

func renderTaskCancelled(msg db.Message) entry {
	var t db.Task
	_ = json.Unmarshal([]byte(msg.Body), &t)

	lines := []string{
		"[task cancelled]",
		"task_id: " + id.Shorten(t.ID),
		"goal: " + t.Goal,
		"reason: " + t.CancellationReason,
	}

	return entry{at: msg.SentAt, from: "system", text: padLines(lines)}
}

func renderAlarmFired(msg db.Message) entry {
	var a db.Alarm
	_ = json.Unmarshal([]byte(msg.Body), &a)

	recurring := a.Recurrence.Valid && a.Recurrence.String != ""

	header := "[one-off alarm fired]"
	if recurring {
		header = "[recurring alarm fired]"
	}

	lines := []string{
		header,
		"alarm_id: " + id.Shorten(a.ID),
		"goal: " + a.Goal,
	}
	if recurring {
		lines = append(lines, "recurrence: "+a.Recurrence.String)
	}
	if a.LastOccurrenceNote.Valid && a.LastOccurrenceNote.String != "" {
		lines = append(lines, "last_time: "+a.LastOccurrenceNote.String)
	}
	lines = append(lines, "MANDATORY: if you already handled this alarm, move on. If you are handling this alarm now, load the alarm skill and follow all instructions meticulously.")

	return entry{at: msg.SentAt, from: "system", text: padLines(lines)}
}

func renderAlarmStale(msg db.Message) entry {
	var a db.Alarm
	_ = json.Unmarshal([]byte(msg.Body), &a)

	lines := []string{
		"[alarm needs rescheduling]",
		"alarm_id: " + id.Shorten(a.ID),
		"goal: " + a.Goal,
		"recurrence: " + a.Recurrence.String,
		"ACTION REQUIRED: call update_alarm with a new next_fire_at",
	}

	return entry{at: msg.SentAt, from: "system", text: padLines(lines)}
}

func renderAlarmCreated(msg db.Message) entry {
	var a db.Alarm
	_ = json.Unmarshal([]byte(msg.Body), &a)

	recurring := a.Recurrence.Valid && a.Recurrence.String != ""

	lines := []string{
		"[alarm created]",
		"alarm_id: " + id.Shorten(a.ID),
		"goal: " + a.Goal,
	}
	if recurring {
		lines = append(lines, "recurrence: "+a.Recurrence.String)
	}
	if a.NextFireAt.Valid {
		lines = append(lines, "fires_at: "+a.NextFireAt.Time.Format("Jan 2, 2006 3:04 PM"))
	}

	return entry{at: msg.SentAt, from: "system", text: padLines(lines)}
}

func renderAlarmUpdated(msg db.Message) entry {
	var u alarms.AlarmUpdated
	_ = json.Unmarshal([]byte(msg.Body), &u)

	lines := []string{
		"[alarm updated]",
		"alarm_id: " + id.Shorten(u.Alarm.ID),
		"goal: " + u.Alarm.Goal,
	}
	if u.Alarm.NextFireAt.Valid {
		lines = append(lines, "next_fire_at: "+u.Alarm.NextFireAt.Time.Format("Jan 2, 2006 3:04 PM"))
	}
	if u.Note != "" {
		lines = append(lines, "note: "+u.Note)
	}

	return entry{at: msg.SentAt, from: "system", text: padLines(lines)}
}

func renderSkillEvent(msg db.Message) entry {
	var s db.Skill
	_ = json.Unmarshal([]byte(msg.Body), &s)

	hasInstall := s.InstallHash.Valid && s.InstallHash.String != ""

	var header string
	lines := []string{}

	switch msg.Kind {
	case "skill_added":
		header = "[skill added]"
		lines = append(lines, header, "name: "+s.Name)
		if hasInstall {
			lines = append(lines, "MANDATORY: call load_skill for this skill and execute every step in ## Install")
		}
	case "skill_removed":
		header = "[skill removed]"
		lines = append(lines, header, "name: "+s.Name, "ask user before cleaning up resources")
	case "skill_changed":
		header = "[skill changed]"
		lines = append(lines, header, "name: "+s.Name)
		if hasInstall {
			lines = append(lines, "MANDATORY: install requirements changed, call load_skill and re-evaluate ## Install idempotently (check existing state, no duplicate alarms)")
		}
	}

	return entry{at: msg.SentAt, from: "system", text: padLines(lines)}
}

func renderTrigger(msg db.Message) entry {
	var t struct {
		Skill string `json:"skill"`
	}
	_ = json.Unmarshal([]byte(msg.Body), &t)

	lines := []string{
		"[trigger] load " + t.Skill + " skill",
		"MANDATORY: load this skill with load_skill and follow all instructions.",
	}

	return entry{at: msg.SentAt, from: "system", text: padLines(lines)}
}

func renderSkillReflexFired(msg db.Message) entry {
	var t struct {
		Skill string `json:"skill"`
		Name  string `json:"name"`
		Meta  string `json:"meta"`
	}
	_ = json.Unmarshal([]byte(msg.Body), &t)

	lines := []string{
		"[skill reflex fired]",
		"skill: " + t.Skill,
		"name:  " + t.Name,
	}

	if t.Meta != "" {
		meta := t.Meta
		if len(meta) > reportTruncateLen {
			meta = meta[:reportTruncateLen] + " [truncated]"
		}
		lines = append(lines, "meta:  "+meta)
	}

	lines = append(lines, "MANDATORY: load this skill with load_skill and follow all instructions.")

	return entry{at: msg.SentAt, from: "system", text: padLines(lines)}
}

func renderToolCall(msg db.Message) entry {
	var tc struct {
		Name   string `json:"name"`
		Input  string `json:"input"`
		Output string `json:"output"`
	}
	_ = json.Unmarshal([]byte(msg.Body), &tc)

	lines := []string{
		"called " + tc.Name,
		"input: " + tc.Input,
		"output: " + tc.Output,
	}

	return entry{at: msg.SentAt, from: "YOU", text: padLines(lines)}
}

func renderMediaProcessed(msg db.Message, database *sql.DB) entry {
	text := "[media described]"

	if msg.ContextStanzaID.Valid && database != nil {
		target, err := db.MessageGet(context.Background(), database, db.MessageGetParams{
			ID: msg.ContextStanzaID.String,
		})
		if err == nil {
			targetSender := resolveContactName(context.Background(), database, target)
			text += " (from " + targetSnippet(target, targetSender) + ")"
		}
	}

	return entry{at: msg.SentAt, from: "system", text: text}
}

func padLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}

	joined := strings.Join(lines, "\n")
	parts := strings.Split(joined, "\n")

	var b strings.Builder
	b.WriteString(parts[0])
	for _, p := range parts[1:] {
		b.WriteByte('\n')
		b.WriteString(pad)
		b.WriteString(p)
	}

	return b.String()
}
