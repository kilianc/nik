package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/kciuffolo/nik/internal/home"
)

// runConfig edits config.yaml by key path, which exists because the
// alternative is editing YAML with sed.
//
// nik-saas configures a managed nik by writing values into this file from
// outside — a model base URL, the sandbox's endpoints — and until now it did
// that with `sed -i` and an anchored line match. That works exactly until it
// does not. On 2026-08-22 an insert after `^shell:` put `docker_image` one
// level too deep, under the `env:` map that the same script had just created:
//
//	shell:
//	  env:
//	    EXA_BASE_URL: https://…
//	    docker_image: nik-shell    # was shell.docker_image
//
// Still valid YAML, so nothing complained. nikd read no shell.docker_image,
// fell back to running the shell tool locally instead of in a container,
// discovered the capsule has no tmux — correctly, it does not need one — and
// exited. Every install after that failed against a daemon that was not there.
//
// A key path and a YAML parser cannot make that mistake. Setting
// `shell.env.EXA_BASE_URL` sets that and only that, whatever the file looks
// like around it.
//
// Scalars only, and deliberately: everything nik-saas needs to write is a
// string, and a command that could write a map is one that can replace a
// subtree by accident.
func runConfig(args []string) {
	flagSet := flag.NewFlagSet("config", flag.ExitOnError)
	homeFlag := flagSet.String("home", "", "workspace directory")

	// Parsed twice on purpose. Go's flag package stops at the first
	// non-flag argument, so `config set a.b c --home /nik` would leave
	// --home unread and silently edit the wrong file. Pull the action out
	// first, then parse what follows it.
	if len(args) == 0 {
		usageConfig()
	}
	action := args[0]
	flagSet.Parse(args[1:])
	rest := flagSet.Args()

	h, err := home.Resolve(*homeFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	path := filepath.Join(h, "config.yaml")

	switch action {
	case "set":
		if len(rest) != 2 {
			fmt.Fprintln(os.Stderr, "usage: nikctl config set <key.path> <value> [--home dir]")
			os.Exit(1)
		}
		if err := configSet(path, rest[0], rest[1]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

	case "get":
		if len(rest) != 1 {
			fmt.Fprintln(os.Stderr, "usage: nikctl config get <key.path> [--home dir]")
			os.Exit(1)
		}
		value, err := configGet(path, rest[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(value)

	default:
		fmt.Fprintf(os.Stderr, "unknown config action %q\n", action)
		usageConfig()
	}
}

func usageConfig() {
	fmt.Fprintln(os.Stderr, "usage: nikctl config {get|set} <key.path> [value] [--home dir]")
	os.Exit(1)
}

// configSet writes one scalar at one key path, creating the maps above it.
//
// Through yaml.Node rather than the Config struct, because a round trip
// through the struct would rewrite the file as the struct sees it: comments
// gone, keys reordered, every default made explicit. This file is read by
// people — the shipped one explains each knob in a trailing comment — and a
// remote configuration step is not a licence to reformat somebody's config.
func configSet(path, keyPath, value string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return fmt.Errorf("%s is empty", path)
	}

	keys := strings.Split(keyPath, ".")
	for _, k := range keys {
		if k == "" {
			return fmt.Errorf("key path %q has an empty segment", keyPath)
		}
	}
	if err := setNode(doc.Content[0], keys, value); err != nil {
		return err
	}

	out, err := marshalDoc(&doc)
	if err != nil {
		return err
	}

	// Written beside the original and renamed over it, so a full disk or a
	// power cut leaves the old config rather than half of a new one. A
	// capsule that loses its config is a capsule that cannot start at all.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

func configGet(path, keyPath string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return "", fmt.Errorf("%s is empty", path)
	}

	node := doc.Content[0]
	for _, k := range strings.Split(keyPath, ".") {
		child := mapValue(node, k)
		if child == nil {
			return "", fmt.Errorf("no %s in %s", keyPath, path)
		}
		node = child
	}
	if node.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("%s is not a single value", keyPath)
	}
	return node.Value, nil
}

// setNode walks the key path, making maps as it goes, and sets the leaf.
func setNode(node *yaml.Node, keys []string, value string) error {
	if node.Kind != yaml.MappingNode {
		return errors.New("config root is not a mapping")
	}

	key := keys[0]
	if len(keys) == 1 {
		if existing := mapValue(node, key); existing != nil {
			if existing.Kind != yaml.ScalarNode {
				return fmt.Errorf("%s already holds a %s, not a value", key, kindName(existing.Kind))
			}
			// Style cleared along with the value. A key that held `""` is
			// quoted in the file, and writing an unquoted URL into a node
			// still marked single-quoted produces `'https://…'` with the
			// quotes as part of the string.
			existing.Value = value
			existing.Tag = "!!str"
			existing.Style = 0
			return nil
		}
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
		return nil
	}

	child := mapValue(node, key)
	if child == nil {
		child = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			child)
	}
	// A key on the path that exists and holds something else is a refusal
	// rather than a replacement: `shell` holding a string means this config
	// is not shaped the way the caller thinks, and writing over it would
	// destroy whatever is there to satisfy an assumption that was wrong.
	if child.Kind == yaml.ScalarNode && child.Value == "" && child.Tag == "!!null" {
		// `env:` with nothing under it parses as a null scalar. That is a
		// map waiting to be filled, not a value somebody set.
		*child = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	if child.Kind != yaml.MappingNode {
		return fmt.Errorf("%s holds a %s, so %s cannot be set under it",
			key, kindName(child.Kind), strings.Join(keys[1:], "."))
	}
	return setNode(child, keys[1:], value)
}

// mapValue returns the value node for a key, or nil.
func mapValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func kindName(k yaml.Kind) string {
	switch k {
	case yaml.MappingNode:
		return "map"
	case yaml.SequenceNode:
		return "list"
	case yaml.ScalarNode:
		return "value"
	default:
		return "something else"
	}
}

// marshalDoc renders the document at the indentation the shipped config uses.
//
// yaml.v3 defaults to four spaces and indents sequences under their key, which
// would rewrite every list in the file the first time any key is set. Two
// spaces matches what nik writes.
func marshalDoc(doc *yaml.Node) ([]byte, error) {
	var sb strings.Builder
	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}
	return []byte(sb.String()), nil
}
