// dbcheck opens nik.db read-only and runs integrity/summary checks.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	dbPath := flag.String("db", "nik.db", "path to the database")
	flag.Parse()

	db, err := sql.Open("sqlite3", "file:"+*dbPath+"?mode=ro")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", *dbPath, err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Println("nik db check")
	fmt.Println("============")
	fmt.Println()

	missing := missingTables(db, []string{
		"contact",
		"conversation",
		"conversation_participant",
		"message",
		"media",
		"message_media",
	})
	if len(missing) > 0 {
		fmt.Printf("canonical schema missing tables: %s\n", strings.Join(missing, ", "))
		fmt.Println("run nik with a canonical schema database (or recreate nik.db in scratch-first mode)")
		os.Exit(1)
	}

	ok := true
	ok = checkRowCounts(db) && ok
	ok = checkOrphanMessages(db) && ok
	ok = checkOrphanContactRefs(db) && ok
	ok = checkOrphanParticipants(db) && ok
	ok = checkOrphanMessageMedia(db) && ok
	ok = checkStaleConversationTimestamps(db) && ok
	ok = checkDuplicateContactJIDs(db) && ok
	ok = checkDuplicateExternalMessageIDs(db) && ok
	ok = checkEmptyTextMessages(db) && ok
	checkMessageKinds(db)
	checkTimeRange(db)

	fmt.Println()
	if !ok {
		fmt.Println("issues found")
		os.Exit(1)
	}
	fmt.Println("all checks passed")
}

func checkRowCounts(db *sql.DB) bool {
	contacts := rowCount(db, "contact")
	conversations := rowCount(db, "conversation")
	messages := rowCount(db, "message")
	media := rowCount(db, "media")
	fmt.Printf("rows: %d contacts, %d conversations, %d messages, %d media\n", contacts, conversations, messages, media)
	return true
}

func checkOrphanMessages(db *sql.DB) bool {
	rows, err := db.Query(`
		SELECT conversation_id, COUNT(*) AS cnt FROM message
		WHERE conversation_id NOT IN (SELECT id FROM conversation)
		GROUP BY conversation_id
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "orphan messages query: %v\n", err)
		return false
	}
	defer rows.Close()

	var total int
	var details []string
	for rows.Next() {
		var conversationID string
		var cnt int
		rows.Scan(&conversationID, &cnt)
		total += cnt
		details = append(details, fmt.Sprintf("  %s  %d", conversationID, cnt))
	}

	if total == 0 {
		fmt.Println("orphan messages (no conversation): 0 OK")
		return true
	}
	fmt.Printf("orphan messages (no conversation): %d\n", total)
	for _, d := range details {
		fmt.Println(d)
	}
	return false
}

func checkOrphanContactRefs(db *sql.DB) bool {
	var cnt int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM message
		WHERE contact_id NOT IN (SELECT id FROM contact)
	`).Scan(&cnt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "orphan contact refs query: %v\n", err)
		return false
	}
	if cnt == 0 {
		fmt.Println("orphan contact refs: 0 OK")
		return true
	}
	fmt.Printf("orphan contact refs: %d\n", cnt)
	return false
}

func checkOrphanParticipants(db *sql.DB) bool {
	rows, err := db.Query(`
		SELECT cp.conversation_id, cp.contact_id
		FROM conversation_participant cp
		LEFT JOIN conversation c ON c.id = cp.conversation_id
		LEFT JOIN contact ct ON ct.id = cp.contact_id
		WHERE c.id IS NULL OR ct.id IS NULL
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "orphan participants query: %v\n", err)
		return false
	}
	defer rows.Close()

	var count int
	var details []string
	for rows.Next() {
		var conversationID, contactID string
		rows.Scan(&conversationID, &contactID)
		count++
		details = append(details, fmt.Sprintf("  conversation=%s contact=%s", conversationID, contactID))
	}

	if count == 0 {
		fmt.Println("orphan participants: 0 OK")
		return true
	}
	fmt.Printf("orphan participants: %d\n", count)
	for _, d := range details {
		fmt.Println(d)
	}
	return false
}

func checkOrphanMessageMedia(db *sql.DB) bool {
	rows, err := db.Query(`
		SELECT mm.message_id, mm.media_id
		FROM message_media mm
		LEFT JOIN message m ON m.id = mm.message_id
		LEFT JOIN media md ON md.id = mm.media_id
		WHERE m.id IS NULL OR md.id IS NULL
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "orphan message_media query: %v\n", err)
		return false
	}
	defer rows.Close()

	var count int
	var details []string
	for rows.Next() {
		var messageID, mediaID string
		rows.Scan(&messageID, &mediaID)
		count++
		details = append(details, fmt.Sprintf("  message=%s media=%s", messageID, mediaID))
	}

	if count == 0 {
		fmt.Println("orphan message_media links: 0 OK")
		return true
	}

	fmt.Printf("orphan message_media links: %d\n", count)
	for _, d := range details {
		fmt.Println(d)
	}
	return false
}

func checkStaleConversationTimestamps(db *sql.DB) bool {
	rows, err := db.Query(`
		SELECT c.id, c.last_message_at, MAX(m.sent_at) AS actual_latest
		FROM conversation c
		JOIN message m ON m.conversation_id = c.id
		GROUP BY c.id, c.last_message_at
		HAVING c.last_message_at IS NULL
		    OR c.last_message_at != MAX(m.sent_at)
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stale timestamps query: %v\n", err)
		return false
	}
	defer rows.Close()

	var stale int
	var details []string
	for rows.Next() {
		var conversationID string
		var conversationTS sql.NullTime
		var actualTS time.Time
		rows.Scan(&conversationID, &conversationTS, &actualTS)
		stale++
		conversationStr := "(null)"
		if conversationTS.Valid {
			conversationStr = conversationTS.Time.Format("2006-01-02 15:04")
		}
		details = append(details, fmt.Sprintf("  %s  conversation=%s actual=%s",
			conversationID, conversationStr, actualTS.Format("2006-01-02 15:04")))
	}

	if stale == 0 {
		fmt.Println("stale conversation timestamps: 0 OK")
		return true
	}
	fmt.Printf("stale conversation timestamps: %d conversations\n", stale)
	for _, d := range details {
		fmt.Println(d)
	}
	return false
}

func checkDuplicateContactJIDs(db *sql.DB) bool {
	rows, err := db.Query(`
		SELECT j.value AS jid, COUNT(*) AS cnt
		FROM contact, json_each(contact.whatsapp_ids) j
		GROUP BY j.value HAVING COUNT(*) > 1
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "duplicate JIDs query: %v\n", err)
		return false
	}
	defer rows.Close()

	var dupes int
	var details []string
	for rows.Next() {
		var jid string
		var cnt int
		rows.Scan(&jid, &cnt)
		dupes++
		details = append(details, fmt.Sprintf("  %s  %d contacts", jid, cnt))
	}

	if dupes == 0 {
		fmt.Println("duplicate contact JIDs: 0 OK")
		return true
	}
	fmt.Printf("duplicate contact JIDs: %d\n", dupes)
	for _, d := range details {
		fmt.Println(d)
	}
	return false
}

func checkDuplicateExternalMessageIDs(db *sql.DB) bool {
	rows, err := db.Query(`
		SELECT platform, external_message_id, COUNT(*) AS cnt
		FROM message
		GROUP BY platform, external_message_id
		HAVING COUNT(*) > 1
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "duplicate external ids query: %v\n", err)
		return false
	}
	defer rows.Close()

	var dupes int
	var details []string
	for rows.Next() {
		var platform, externalID string
		var cnt int
		rows.Scan(&platform, &externalID, &cnt)
		dupes++
		details = append(details, fmt.Sprintf("  %s/%s  %d", platform, externalID, cnt))
	}

	if dupes == 0 {
		fmt.Println("duplicate external message ids: 0 OK")
		return true
	}
	fmt.Printf("duplicate external message ids: %d\n", dupes)
	for _, d := range details {
		fmt.Println(d)
	}
	return false
}

func checkEmptyTextMessages(db *sql.DB) bool {
	var cnt int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM message WHERE kind = 'text' AND body = ''
	`).Scan(&cnt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "empty text messages query: %v\n", err)
		return false
	}
	if cnt == 0 {
		fmt.Println("empty text messages: 0 OK")
		return true
	}
	fmt.Printf("empty text messages: %d\n", cnt)
	return false
}

func checkMessageKinds(db *sql.DB) {
	rows, err := db.Query(`
		SELECT kind, COUNT(*) AS cnt FROM message
		GROUP BY kind ORDER BY cnt DESC
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "message kinds query: %v\n", err)
		return
	}
	defer rows.Close()

	var parts []string
	for rows.Next() {
		var kind string
		var cnt int
		rows.Scan(&kind, &cnt)
		parts = append(parts, fmt.Sprintf("%s=%d", kind, cnt))
	}
	fmt.Printf("kinds: %s\n", strings.Join(parts, " "))
}

func checkTimeRange(db *sql.DB) {
	var minTS, maxTS sql.NullString
	err := db.QueryRow(`
		SELECT MIN(sent_at), MAX(sent_at) FROM message
	`).Scan(&minTS, &maxTS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "time range query: %v\n", err)
		return
	}
	if !minTS.Valid || !maxTS.Valid {
		fmt.Println("range: (no messages)")
		return
	}
	fmt.Printf("range: %s .. %s\n", minTS.String[:10], maxTS.String[:10])
}

func rowCount(db *sql.DB, table string) int {
	var cnt int
	err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&cnt)
	if err != nil {
		return 0
	}

	return cnt
}

func missingTables(db *sql.DB, tables []string) []string {
	var missing []string
	for _, table := range tables {
		var name string
		err := db.QueryRow(`
			SELECT name FROM sqlite_master
			WHERE type = 'table' AND name = ?
		`, table).Scan(&name)
		if err != nil {
			missing = append(missing, table)
		}
	}

	return missing
}
