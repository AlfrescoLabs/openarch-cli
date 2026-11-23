package guides

import (
	"strings"
	"testing"
)

func TestHxQL(t *testing.T) {
	if !strings.HasPrefix(HxQL, "# HxQL guide\n") {
		t.Error("hxql.md must start with its title")
	}
	for i, b := range []byte(HxQL) {
		if b > 127 {
			t.Errorf("hxql.md has a non-ASCII byte at offset %d", i)
			break
		}
	}
}
