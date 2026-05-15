package prompt

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
	"github.com/kciuffolo/nik/internal/skills"
)

type BrainData struct {
	Model       string
	BannedWords []string
	TableList   string
	WorkerTools []string
	NikTools    []string
	Skills      SkillData
}

type SkillData struct {
	Preloaded []PreloadedSkill
	Available []SkillSummary
}

type PreloadedSkill struct {
	Name    string
	Tools   []string
	Content string
}

type SkillSummary struct {
	Name    string
	Summary string
	Tools   []string
}

type InputData struct {
	Recall   string
	Timeline string
}

type TaskData struct {
	ShellEnv    string
	TableList   string
	TokenTraps  TokenTraps
	Tools       []ToolDoc
	Skills      SkillData
	Plan        string
	Timeout     string
	MaxRounds   int
	Profile     string
	BannedWords []string
}

type ToolDoc struct {
	Name        string
	Description string
}

type TokenTraps struct {
	Dated []DatedDir
	Large []LargeFile
	Dense []DenseDir
}

type DatedDir struct {
	Path   string
	Latest string
}

type LargeFile struct {
	Path string
	Size int64
}

func (f LargeFile) SizeKB() int64 {
	return (f.Size + 1023) / 1024
}

type DenseDir struct {
	Path  string
	Count int
}

func (r *Renderer) BuildBrainData(workerToolNames []string, toolDefs []llm.ToolDef) BrainData {
	cfg := r.cfg

	data := BrainData{
		Model:       cfg.Models.Main.Model,
		BannedWords: cfg.BannedWords,
	}

	data.TableList = db.TableList()

	if len(workerToolNames) > 0 {
		workerSet := make(map[string]bool, len(workerToolNames))
		for _, name := range workerToolNames {
			workerSet[name] = true
		}
		data.WorkerTools = workerToolNames

		var nikOnly []string
		for _, def := range toolDefs {
			if !workerSet[def.Name] {
				nikOnly = append(nikOnly, def.Name)
			}
		}
		data.NikTools = nikOnly
	}

	data.Skills = LoadSkills(nil, skills.Sources(cfg.Home)...)

	return data
}

func BuildTaskData(cfg *config.Config, t db.Task, tools []llm.ToolDef) TaskData {
	profile := cfg.Task.Profile

	var shellEnv string
	if cfg.Shell.DockerImage != "" {
		shellEnv = "Docker container (Debian bookworm)"
	}

	var toolDocs []ToolDoc
	for _, td := range tools {
		toolDocs = append(toolDocs, ToolDoc{Name: td.Name, Description: td.Description})
	}

	data := TaskData{
		ShellEnv:   shellEnv,
		TableList:  db.TableList(),
		TokenTraps: ScanTokenTraps(cfg.Home),
		Tools:      toolDocs,
		Skills:     LoadSkills(toolDocs, skills.Sources(cfg.Home)...),
		Plan:       t.Plan,
		Timeout:    cfg.Task.TimeoutOrDefault().String(),
		MaxRounds:  cfg.Task.MaxRoundsOrDefault(),
		Profile:    profile,
	}

	if profile == "nik" {
		data.BannedWords = cfg.BannedWords
	}

	return data
}

// LoadSkills loads preloaded and available skills from the given sources.
// When filterTools is non-nil, preloaded skills are included only if at least
// one of their declared tools is present in filterTools.
func LoadSkills(filterTools []ToolDoc, srcs ...fs.FS) SkillData {
	var available map[string]bool
	if len(filterTools) > 0 {
		available = make(map[string]bool, len(filterTools))
		for _, td := range filterTools {
			available[td.Name] = true
		}
	}

	var sd SkillData

	preloaded, err := skills.PreloadedSkills(srcs...)
	if err != nil {
		slog.Warn("load preloaded skills", "pkg", "prompt", "error", err)
	}
	for _, s := range preloaded {
		if available != nil && len(s.Tools) > 0 && !anyToolAvailable(s.Tools, available) {
			continue
		}
		sd.Preloaded = append(sd.Preloaded, PreloadedSkill{
			Name:    s.Name,
			Tools:   s.Tools,
			Content: s.Content,
		})
	}

	summaries, err := skills.ListSkills(srcs...)
	if err != nil {
		slog.Warn("load skill index", "pkg", "prompt", "error", err)
	}
	for _, s := range summaries {
		if s.Preload {
			continue
		}
		sd.Available = append(sd.Available, SkillSummary{
			Name:    s.Name,
			Summary: s.Summary,
			Tools:   s.Tools,
		})
	}

	return sd
}

func ScanTokenTraps(home string) TokenTraps {
	const (
		sizeThreshold  = 50 * 1024
		countThreshold = 30
	)

	skipDirs := map[string]bool{
		".git":     true,
		".cursor":  true,
		".gocache": true,
		".tmp":     true,
		"vendor":   true,
		"media":    true,
		"backups":  true,
		"tmp":      true,
	}

	var tt TokenTraps

	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path != home && d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
		}

		if !d.IsDir() {
			return nil
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}

		rel, err := filepath.Rel(home, path)
		if err != nil {
			return nil
		}
		if rel == "." {
			rel = ""
		}
		rel = rel + string(filepath.Separator)

		latestPath := filepath.Join(path, "latest.md")
		if _, err := os.Lstat(latestPath); err == nil {
			target, _ := os.Readlink(latestPath)
			if target == "" {
				target = "latest.md"
			}
			tt.Dated = append(tt.Dated, DatedDir{Path: rel, Latest: target})
			return filepath.SkipDir
		}

		fileCount := 0
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			fileCount++

			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.Size() > sizeThreshold {
				frel, err := filepath.Rel(home, filepath.Join(path, entry.Name()))
				if err != nil {
					continue
				}
				tt.Large = append(tt.Large, LargeFile{Path: frel, Size: info.Size()})
			}
		}

		if fileCount > countThreshold {
			tt.Dense = append(tt.Dense, DenseDir{Path: rel, Count: fileCount})
		}

		return nil
	})

	return tt
}

func anyToolAvailable(tools []string, available map[string]bool) bool {
	for _, t := range tools {
		if available[t] {
			return true
		}
	}
	return false
}
