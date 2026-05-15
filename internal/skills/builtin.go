package skills

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

//go:embed all:builtin
var builtinFS embed.FS

var builtinOnce = sync.OnceValue(func() fs.FS {
	sub, err := fs.Sub(builtinFS, "builtin")
	if err != nil {
		panic("skills: sub builtin: " + err.Error())
	}
	return sub
})

// BuiltinFS returns the embedded built-in skills, rooted so top-level
// entries are skill directory names (e.g. "journal/SKILL.md").
func BuiltinFS() fs.FS {
	return builtinOnce()
}

// Sources returns the ordered list of skill sources: built-in first, then
// the user workspace at <home>/skills. Later sources override earlier ones
// by skill name. The workspace source returns no entries when its directory
// is missing -- callers must tolerate that.
func Sources(home string) []fs.FS {
	return []fs.FS{
		BuiltinFS(),
		os.DirFS(filepath.Join(home, "skills")),
	}
}
