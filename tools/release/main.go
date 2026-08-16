// release cuts a stable release: bumps VERSION, commits it, tags it, pushes
// both. the release workflow refuses any tag that disagrees with the VERSION
// committed alongside it, and that is two facts to keep in sync by hand —
// one too many at 1am, which is when releases get cut.
//
//	make release                      # patch bump
//	make release ARGS="-bump minor"   # minor bump
//	make release ARGS="-dry-run"      # print what would happen, touch nothing
//	make release ARGS="-no-ci"        # skip make ci, you already ran it
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	versionFile = "VERSION"
	actionsURL  = "https://github.com/kilianc/nik/actions/workflows/release.yaml"
)

type version struct {
	major int
	minor int
	patch int
}

func (v version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch)
}

func (v version) after(o version) bool {
	if v.major != o.major {
		return v.major > o.major
	}
	if v.minor != o.minor {
		return v.minor > o.minor
	}
	return v.patch > o.patch
}

func parseVersion(s string) (version, error) {
	digits, ok := strings.CutPrefix(strings.TrimSpace(s), "v")
	if !ok {
		return version{}, fmt.Errorf("parse version %q: want vMAJOR.MINOR.PATCH", strings.TrimSpace(s))
	}

	parts := strings.Split(digits, ".")
	if len(parts) != 3 {
		return version{}, fmt.Errorf("parse version %q: want vMAJOR.MINOR.PATCH", strings.TrimSpace(s))
	}

	var out [3]int
	for i, part := range parts {
		// leading zeros are rejected: v0.02.0 and v0.2.0 are the same release
		// to anyone reading, and two different tags to git.
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return version{}, fmt.Errorf("parse version %q: bad component %q", strings.TrimSpace(s), part)
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return version{}, fmt.Errorf("parse version %q: bad component %q", strings.TrimSpace(s), part)
		}
		out[i] = n
	}

	return version{major: out[0], minor: out[1], patch: out[2]}, nil
}

func bump(v version, level string) (version, error) {
	switch level {
	case "major":
		return version{major: v.major + 1}, nil
	case "minor":
		return version{major: v.major, minor: v.minor + 1}, nil
	case "patch":
		return version{major: v.major, minor: v.minor, patch: v.patch + 1}, nil
	}
	return version{}, fmt.Errorf("bump %q: want major, minor or patch", level)
}

// gitBin is resolved once: in this workspace a wrapper on PATH can inject
// commit trailers the repo forbids, and a release commit is permanent.
func gitBin() string {
	if _, err := os.Stat("/usr/bin/git"); err == nil {
		return "/usr/bin/git"
	}
	return "git"
}

func git(args ...string) (string, error) {
	out, err := exec.Command(gitBin(), args...).Output()
	if err != nil {
		// git's own message is the useful half — "exit status 128" alone
		// sends you looking in the wrong place, and around here a failed
		// fetch is usually a Touch ID prompt nobody confirmed.
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "release: "+format+"\n", args...)
	os.Exit(1)
}

// guard refuses every way to ship a release nobody can reproduce: a tag on a
// commit that is not on main, a VERSION bump on top of uncommitted work, or a
// tag pointing at something origin has never seen.
func guard() {
	branch, err := git("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		die("%v", err)
	}
	if branch != "main" {
		die("releases are cut from main, not %s", branch)
	}

	dirty, err := git("status", "--porcelain")
	if err != nil {
		die("%v", err)
	}
	if dirty != "" {
		die("working tree is dirty; commit or stash first")
	}

	if _, err := git("fetch", "origin", "main"); err != nil {
		die("%v", err)
	}

	local, err := git("rev-parse", "HEAD")
	if err != nil {
		die("%v", err)
	}
	remote, err := git("rev-parse", "origin/main")
	if err != nil {
		die("%v", err)
	}
	if local != remote {
		die("main and origin/main disagree; pull or push before releasing")
	}
}

func tagTaken(tag string) bool {
	if _, err := git("rev-parse", "-q", "--verify", "refs/tags/"+tag); err == nil {
		return true
	}
	remote, err := git("ls-remote", "--tags", "origin", "refs/tags/"+tag)
	if err != nil {
		die("%v", err)
	}
	return remote != ""
}

func confirm(tag string) bool {
	fmt.Printf("release: tag %s and push to origin? [y/N] ", tag)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

func main() {
	level := flag.String("bump", "patch", "component to bump: major, minor or patch")
	explicit := flag.String("version", "", "cut this exact version instead of bumping")
	dryRun := flag.Bool("dry-run", false, "print what would happen, touch nothing")
	noCI := flag.Bool("no-ci", false, "skip make ci")
	flag.Parse()

	root, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		die("%v", err)
	}
	if err := os.Chdir(root); err != nil {
		die("cd %s: %v", root, err)
	}

	data, err := os.ReadFile(versionFile)
	if err != nil {
		die("read %s: %v", versionFile, err)
	}
	current, err := parseVersion(string(data))
	if err != nil {
		die("%v", err)
	}

	var next version
	if *explicit != "" {
		next, err = parseVersion(*explicit)
	} else {
		next, err = bump(current, *level)
	}
	if err != nil {
		die("%v", err)
	}
	if !next.after(current) {
		die("%s does not come after %s", next, current)
	}

	// after the arithmetic, so a typo fails before any of this touches the
	// network.
	guard()

	if tagTaken(next.String()) {
		die("tag %s already exists", next)
	}

	head, err := git("rev-parse", "--short=7", "HEAD")
	if err != nil {
		die("%v", err)
	}
	fmt.Printf("release: %s → %s at %s\n", current, next, head)

	if *dryRun {
		fmt.Println("release: dry run, nothing changed")
		return
	}

	// the workflow runs make ci too, but it runs it after the tag exists. a
	// tag is awkward to retract and a failed release is a confusing artifact,
	// so the cheap thing is to find out here.
	if !*noCI {
		fmt.Println("release: running make ci (skip with -no-ci)")
		if err := run("make", "ci"); err != nil {
			die("%v", err)
		}
	}

	if !confirm(next.String()) {
		die("aborted")
	}

	// the commit goes first and on its own. if origin has moved since the
	// fetch above, the push is rejected here — before a tag exists — and the
	// worst case is a local VERSION bump to reset, rather than a published
	// tag pointing at a commit the workflow will refuse.
	if err := os.WriteFile(versionFile, []byte(next.String()+"\n"), 0o644); err != nil {
		die("write %s: %v", versionFile, err)
	}

	steps := [][]string{
		{"add", versionFile},
		{"commit", "-m", "chore: release " + next.String()},
		{"push", "origin", "main"},
		{"tag", "-a", next.String(), "-m", "Release " + next.String()},
		{"push", "origin", "refs/tags/" + next.String()},
	}
	for _, step := range steps {
		if err := run(gitBin(), step...); err != nil {
			fmt.Fprintf(os.Stderr, "release: %s is bumped to %s locally; undo it before trying again\n", versionFile, next)
			die("%v", err)
		}
	}

	fmt.Printf("release: %s pushed. the release workflow is building:\n  %s\n", next, actionsURL)
}
