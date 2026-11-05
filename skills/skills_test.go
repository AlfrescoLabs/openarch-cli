package skills

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestSkillFrontMatter(t *testing.T) {
	if !bytes.HasPrefix(SKILL, []byte("---\nname: "+Name+"\ndescription: ")) {
		t.Fatalf("SKILL.md must start with front matter that has name %q and a description", Name)
	}
	for i, b := range SKILL {
		if b > 127 {
			t.Fatalf("SKILL.md has a non-ASCII byte at offset %d", i)
		}
	}
	desc := strings.TrimPrefix(strings.SplitN(string(SKILL), "\n", 4)[2], "description: ")
	if len(desc) > 1024 {
		t.Errorf("description is longer than 1024 characters")
	}
}

func TestInstall(t *testing.T) {
	dir := t.TempDir()
	path, action, err := Install(dir, false)
	if err != nil || action != Create {
		t.Fatalf("first install = %q, %v", action, err)
	}
	if _, action, err = Install(dir, false); err != nil || action != Unchanged {
		t.Fatalf("second install = %q, %v", action, err)
	}
	if err := os.WriteFile(path, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err = Install(dir, false); !errors.Is(err, ErrChanged) {
		t.Fatalf("install over an edited file: err = %v, want ErrChanged", err)
	}
	if _, action, err = Install(dir, true); err != nil || action != Update {
		t.Fatalf("install with force = %q, %v", action, err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, SKILL) {
		t.Errorf("SKILL.md was not replaced")
	}
}
