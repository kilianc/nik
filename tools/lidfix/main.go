// lidfix merges duplicate contacts and deduplicates conversation_participant
// rows caused by WhatsApp's dual JID system (LID vs phone-based JID).
//
// it reads the whatsmeow_lid_map table from wapp_session.db to build the
// LID<->phone mapping, then:
//  1. merges contacts that share the same person (one with LID, one with phone JID)
//  2. deduplicates conversation_participant rows by keeping one per (conversation_id, contact_id)
//
// use -dry-run to preview changes without writing.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type lidMapping struct {
	lid string
	pn  string
}

func main() {
	dbPath := flag.String("db", "nik.db", "path to nik database")
	sessionPath := flag.String("session-db", "wapp_session.db", "path to whatsapp session database")
	dryRun := flag.Bool("dry-run", false, "preview changes without writing")
	flag.Parse()

	sessionDB, err := sql.Open("sqlite3", "file:"+*sessionPath+"?mode=ro")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", *sessionPath, err)
		os.Exit(1)
	}
	defer sessionDB.Close()

	mappings, err := loadLIDMappings(sessionDB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load lid mappings: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("loaded %d LID<->PN mappings from %s\n", len(mappings), *sessionPath)

	nikDB, err := sql.Open("sqlite3", "file:"+*dbPath+"?_foreign_keys=0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", *dbPath, err)
		os.Exit(1)
	}
	defer nikDB.Close()

	merged := mergeContacts(nikDB, mappings, *dryRun)
	fmt.Printf("merged %d duplicate contacts\n", merged)

	deduped := deduplicateParticipants(nikDB, *dryRun)
	fmt.Printf("deduped %d participant rows\n", deduped)

	if *dryRun {
		fmt.Println("\ndry run — no changes written")
	}
}

func loadLIDMappings(db *sql.DB) ([]lidMapping, error) {
	rows, err := db.Query(`SELECT lid, pn FROM whatsmeow_lid_map`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mappings []lidMapping
	for rows.Next() {
		var m lidMapping
		err = rows.Scan(&m.lid, &m.pn)
		if err != nil {
			return nil, err
		}
		mappings = append(mappings, m)
	}

	return mappings, rows.Err()
}

func mergeContacts(db *sql.DB, mappings []lidMapping, dryRun bool) int {
	merged := 0

	for _, m := range mappings {
		pnJID := m.pn + "@s.whatsapp.net"

		// LIDs can appear with device suffixes (e.g. 219971061866633:12@lid),
		// so we find all contacts whose whatsapp_ids contain any JID starting
		// with this LID base number and ending with @lid.
		lidContactIDs := contactsByLIDPrefix(db, m.lid)
		pnContactID := contactByJID(db, pnJID)

		// collect all distinct contact IDs that belong to this person
		allIDs := make(map[string]struct{})
		if pnContactID != "" {
			allIDs[pnContactID] = struct{}{}
		}
		for _, id := range lidContactIDs {
			allIDs[id] = struct{}{}
		}

		if len(allIDs) < 2 {
			continue
		}

		// pick the one with a name as the keeper
		keepID := ""
		for id := range allIDs {
			if contactName(db, id) != "" {
				keepID = id
				break
			}
		}
		if keepID == "" {
			if pnContactID != "" {
				keepID = pnContactID
			} else {
				for id := range allIDs {
					keepID = id
					break
				}
			}
		}

		for dropID := range allIDs {
			if dropID == keepID {
				continue
			}

			fmt.Printf("  merge: keep=%s drop=%s (lid=%s pn=%s)\n", keepID, dropID, m.lid, pnJID)

			if dryRun {
				merged++
				continue
			}

			tx, err := db.Begin()
			if err != nil {
				fmt.Fprintf(os.Stderr, "  begin tx: %v\n", err)
				continue
			}

			// copy all whatsapp_ids from the dropped contact to the keeper
			copyWhatsAppIDs(tx, keepID, dropID)

			repoint(tx, "message", "contact_id", dropID, keepID)
			repoint(tx, "conversation_participant", "contact_id", dropID, keepID)
			repoint(tx, "alarm", "origin_contact_id", dropID, keepID)

			_, err = tx.Exec(`DELETE FROM contact WHERE id = ?`, dropID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  delete contact %s: %v\n", dropID, err)
				tx.Rollback()
				continue
			}

			err = tx.Commit()
			if err != nil {
				fmt.Fprintf(os.Stderr, "  commit: %v\n", err)
				continue
			}

			merged++
		}
	}

	return merged
}

func deduplicateParticipants(db *sql.DB, dryRun bool) int {
	rows, err := db.Query(`
		SELECT conversation_id, contact_id, COUNT(*) AS cnt
		FROM conversation_participant
		GROUP BY conversation_id, contact_id
		HAVING cnt > 1
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "find duplicate participants: %v\n", err)
		return 0
	}
	defer rows.Close()

	type dupKey struct {
		conversationID string
		contactID      string
	}

	var dups []dupKey
	for rows.Next() {
		var k dupKey
		var cnt int
		rows.Scan(&k.conversationID, &k.contactID, &cnt)
		dups = append(dups, k)
		fmt.Printf("  dup: conversation=%s contact=%s count=%d\n", k.conversationID, k.contactID, cnt)
	}

	if dryRun {
		return len(dups)
	}

	deduped := 0
	for _, k := range dups {
		_, err := db.Exec(`
			DELETE FROM conversation_participant
			WHERE conversation_id = ? AND contact_id = ?
			  AND rowid NOT IN (
			    SELECT rowid FROM conversation_participant
			    WHERE conversation_id = ? AND contact_id = ?
			    ORDER BY updated_at DESC
			    LIMIT 1
			  )
		`, k.conversationID, k.contactID, k.conversationID, k.contactID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  dedup participant %s/%s: %v\n", k.conversationID, k.contactID, err)
			continue
		}
		deduped++
	}

	return deduped
}

func contactByJID(db *sql.DB, jid string) string {
	var id string
	err := db.QueryRow(`
		SELECT id FROM contact
		WHERE EXISTS (SELECT 1 FROM json_each(whatsapp_ids) WHERE value = ?)
	`, jid).Scan(&id)
	if err != nil {
		return ""
	}
	return id
}

// contactsByLIDPrefix finds all contacts with a whatsapp_id that starts with
// the given LID base number and ends with @lid, matching both bare LIDs
// (e.g. 219971061866633@lid) and device-suffixed LIDs (e.g. 219971061866633:12@lid).
func contactsByLIDPrefix(db *sql.DB, lidBase string) []string {
	pattern := lidBase + "%@lid"
	rows, err := db.Query(`
		SELECT DISTINCT contact.id FROM contact, json_each(contact.whatsapp_ids) j
		WHERE j.value LIKE ?
	`, pattern)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids
}

func contactName(db *sql.DB, id string) string {
	var name string
	err := db.QueryRow(`SELECT name FROM contact WHERE id = ?`, id).Scan(&name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(name)
}

func copyWhatsAppIDs(tx *sql.Tx, keepID, dropID string) {
	rows, err := tx.Query(`
		SELECT j.value FROM contact, json_each(contact.whatsapp_ids) j
		WHERE contact.id = ?
	`, dropID)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var jid string
		rows.Scan(&jid)
		addJIDToContact(tx, keepID, jid)
	}
}

func addJIDToContact(tx *sql.Tx, contactID, jid string) {
	_, _ = tx.Exec(`
		UPDATE contact
		SET whatsapp_ids = json_insert(whatsapp_ids, '$[#]', ?2),
		    updated_at = datetime('now')
		WHERE id = ?1
		  AND NOT EXISTS (SELECT 1 FROM json_each(whatsapp_ids) WHERE value = ?2)
	`, contactID, jid)
}

func repoint(tx *sql.Tx, table, column, fromID, toID string) {
	query := fmt.Sprintf(`UPDATE %s SET %s = ? WHERE %s = ?`, table, column, column)
	_, err := tx.Exec(query, toID, fromID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  repoint %s.%s %s->%s: %v\n", table, column, fromID, toID, err)
	}
}
