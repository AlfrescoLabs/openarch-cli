// Package skills holds the Agent Skills that teach AI agents to use the CLI.
// The binary embeds them, so the skills always match the commands of the same
// release. Each skill is a folder with a SKILL.md.
package skills

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
)

//go:embed */SKILL.md
var files embed.FS

// Skill is one embedded skill.
type Skill struct {
	// Name is the folder name, the same as the name in the front matter.
	Name    string
	Content []byte
}

// All are the embedded skills, sorted by name.
var All = load()

func load() []Skill {
	paths, err := fs.Glob(files, "*/SKILL.md")
	if err != nil {
		panic("skills: " + err.Error())
	}
	all := make([]Skill, len(paths))
	for i, p := range paths {
		b, err := files.ReadFile(p)
		if err != nil {
			panic("skills: " + err.Error())
		}
		all[i] = Skill{Name: path.Dir(p), Content: b}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	return all
}

// Get returns the skill with this name.
func Get(name string) (Skill, bool) {
	for _, s := range All {
		if s.Name == name {
			return s, true
		}
	}
	return Skill{}, false
}

// Folders are the skill folders that Install writes to, relative to a project
// folder: .agents/skills is read by the agents that follow the cross-client
// convention of Agent Skills, .claude/skills by Claude Code.
var Folders = []string{filepath.Join(".agents", "skills"), filepath.Join(".claude", "skills")}

// File actions, the same words as the files that init writes.
const (
	Create    = "create"
	Update    = "update"
	Unchanged = "unchanged"
)

// ErrChanged means that an installed SKILL.md differs from this release and
// force was not set.
var ErrChanged = errors.New("SKILL.md exists and differs from this release")

// File is one SKILL.md that Install wrote or found up to date.
type File struct {
	Skill  string `json:"skill"`
	Path   string `json:"path"`
	Action string `json:"action"`
}

// Install writes every skill to every folder of Folders under dir. A file
// that differs from this release is replaced only with force. Without force,
// Install writes nothing when one file differs, and returns its path with
// ErrChanged.
func Install(dir string, force bool) ([]File, error) {
	var out []File
	for _, folder := range Folders {
		for _, s := range All {
			p := filepath.Join(dir, folder, s.Name, "SKILL.md")
			old, err := os.ReadFile(p)
			f := File{Skill: s.Name, Path: p}
			switch {
			case err == nil && bytes.Equal(old, s.Content):
				f.Action = Unchanged
			case err == nil && !force:
				return []File{f}, ErrChanged
			case err == nil:
				f.Action = Update
			case errors.Is(err, fs.ErrNotExist):
				f.Action = Create
			default:
				return []File{f}, err
			}
			out = append(out, f)
		}
	}
	for _, f := range out {
		if f.Action == Unchanged {
			continue
		}
		s, _ := Get(f.Skill)
		if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
			return out, err
		}
		if err := os.WriteFile(f.Path, s.Content, 0o644); err != nil {
			return out, err
		}
	}
	return out, nil
}
