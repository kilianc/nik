package main

import (
	"flag"
	"strings"
)

// parseFlags parses args with this command's flags allowed anywhere, not only
// in front of the first positional.
//
// Go's flag package stops at the first non-flag argument, so
// `nikctl secrets write openai_key --home /nik` never reads --home at all: it
// lands in the positional list, and the command then either resolves the wrong
// home or exits on an argument count it did not expect. Nobody types it that
// way once and learns; a trailing flag is how every operator writes a line
// they are editing, and the failure it produces points nowhere near the cause.
//
// So the flags this command defines are hoisted to the front, everything else
// keeps its order behind a `--`, and flag.Parse sees the shape it wants.
//
// Only defined flags move, which is the whole reason this is safe:
// `nikctl shell ls -la` still hands -la to the shell rather than dying on an
// unknown flag, and a mistyped flag in front of the positionals still gets
// flag.Parse's "not defined" rather than being quietly swallowed as an
// argument.
func parseFlags(fs *flag.FlagSet, args []string) {
	var flags, positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// The terminator means the rest is arguments, whatever it looks like.
		// It is re-emitted below, so it keeps meaning that.
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}

		if arg == "-" || !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}

		f := fs.Lookup(flagName(arg))
		if f == nil {
			// Undefined and in front of every positional: a typo, and
			// flag.Parse is the right thing to say so. Undefined and after
			// one: somebody's argument, which is what it already is today.
			if len(positional) > 0 {
				positional = append(positional, arg)
				continue
			}
			flags = append(flags, arg)
			continue
		}

		flags = append(flags, arg)

		// A value only travels separately when it was written separately and
		// the flag takes one at all — hoisting the token after --errors would
		// eat a positional.
		if strings.Contains(arg, "=") || isBoolFlag(f) {
			continue
		}
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}

	if len(positional) > 0 {
		flags = append(flags, "--")
		flags = append(flags, positional...)
	}

	fs.Parse(flags)
}

// flagName is the name inside a flag token: -home, --home and --home=/nik are
// all `home`.
func flagName(arg string) string {
	name := strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-")
	if i := strings.Index(name, "="); i >= 0 {
		name = name[:i]
	}
	return name
}

func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}
