package skills

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillFrontMatter(t *testing.T) {
	if len(All) < 2 {
		t.Fatalf("skills = %d, want openarch and openarch-search", len(All))
	}
	for _, s := range All {
		if !bytes.HasPrefix(s.Content, []byte("---\nname: "+s.Name+"\ndescription: ")) {
			t.Errorf("%s/SKILL.md must start with front matter that has name %q and a description", s.Name, s.Name)
		}
		for i, b := range s.Content {
			if b > 127 {
				t.Errorf("%s/SKILL.md has a non-ASCII byte at offset %d", s.Name, i)
				break
			}
		}
		desc := strings.TrimPrefix(strings.SplitN(string(s.Content), "\n", 4)[2], "description: ")
		if len(desc) > 1024 {
			t.Errorf("%s: description is longer than 1024 characters", s.Name)
		}
		if len(s.Name) > 64 {
			t.Errorf("%s: name is longer than 64 characters", s.Name)
		}
	}
	if _, ok := Get("openarch"); !ok {
		t.Error("no openarch skill")
	}
}

func TestInstall(t *testing.T) {
	dir := t.TempDir()
	files, err := Install(dir, false)
	if err != nil || len(files) != len(All)*len(Folders) {
		t.Fatalf("first install = %v, %v", files, err)
	}
	for _, f := range files {
		if f.Action != Create {
			t.Errorf("%s: %s, want create", f.Path, f.Action)
		}
	}
	for _, folder := range []string{".agents/skills", ".claude/skills"} {
		if _, err := os.Stat(filepath.Join(dir, folder, "openarch", "SKILL.md")); err != nil {
			t.Error(err)
		}
	}
	if files, err = Install(dir, false); err != nil || files[0].Action != Unchanged {
		t.Fatalf("second install = %v, %v", files, err)
	}
	edited := filepath.Join(dir, ".claude", "skills", "openarch", "SKILL.md")
	if err := os.WriteFile(edited, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A missing file next to an edited one is not written without force.
	missing := filepath.Join(dir, ".agents", "skills", "openarch", "SKILL.md")
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	if files, err = Install(dir, false); !errors.Is(err, ErrChanged) || files[0].Path != edited {
		t.Fatalf("install over an edited file: %v, %v, want ErrChanged", files, err)
	}
	if _, err := os.Stat(missing); err == nil {
		t.Error("Install wrote a file before it found the edited one")
	}
	if _, err = Install(dir, true); err != nil {
		t.Fatalf("install with force: %v", err)
	}
	s, _ := Get("openarch")
	for _, p := range []string{edited, missing} {
		if got, _ := os.ReadFile(p); !bytes.Equal(got, s.Content) {
			t.Errorf("%s was not written", p)
		}
	}
}
