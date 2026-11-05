// Package skills holds the Agent Skill that teaches AI coding agents to use
// the CLI. The binary embeds it, so the skill always matches the commands of
// the same release.
package skills

import (
	"bytes"
	_ "embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Name is the skill name, as in the SKILL.md front matter.
const Name = "openarch"

// SKILL is the content of openarch/SKILL.md.
//
//go:embed openarch/SKILL.md
var SKILL []byte

// File actions, the same words as the files that init writes.
const (
	Create    = "create"
	Update    = "update"
	Unchanged = "unchanged"
)

// ErrChanged means that the installed SKILL.md differs from this release and
// force was not set.
var ErrChanged = errors.New("SKILL.md exists and differs from this release")

// Path is where Install writes the skill in a project folder. Claude Code and
// other agents that read Agent Skills find it there.
func Path(dir string) string {
	return filepath.Join(dir, ".claude", "skills", Name, "SKILL.md")
}

// Install writes SKILL.md under dir and returns the file path and the action.
// A different file is replaced only with force.
func Install(dir string, force bool) (path, action string, err error) {
	path = Path(dir)
	old, err := os.ReadFile(path)
	switch {
	case err == nil && bytes.Equal(old, SKILL):
		return path, Unchanged, nil
	case err == nil && !force:
		return path, "", ErrChanged
	case err == nil:
		action = Update
	case errors.Is(err, fs.ErrNotExist):
		action = Create
	default:
		return path, "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path, "", err
	}
	if err := os.WriteFile(path, SKILL, 0o644); err != nil {
		return path, "", err
	}
	return path, action, nil
}
