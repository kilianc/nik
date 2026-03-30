package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/skills"
)

func main() {
	home := flag.String("home", "workspace", "nik home directory")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: trigger [-home dir] <skill_name>")
		os.Exit(1)
	}
	skillName := flag.Arg(0)

	cfg, err := config.Load(*home)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	dirs := []string{cfg.SkillsPath(), cfg.WorkspaceSkillsPath()}
	found := false

	summaries, err := skills.ListSkills(dirs...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list skills: %v\n", err)
		os.Exit(1)
	}
	for _, s := range summaries {
		if s.Name == skillName {
			found = true
			break
		}
	}
	if !found {
		fmt.Fprintf(os.Stderr, "skill %q not found\n", skillName)
		os.Exit(1)
	}

	convID := cfg.PrivilegedIDs()[0]

	conn, err := db.Open(cfg.DBPath(), cfg.TZ())
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	err = db.SystemMessageInsert(context.Background(), conn, db.SystemMessageParams{
		ConversationID: convID,
		Kind:           "trigger",
		Body:           map[string]string{"skill": skillName},
		SentAt:         time.Now().UTC(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "insert trigger: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("triggered %q in conversation %s\n", skillName, convID)
}
