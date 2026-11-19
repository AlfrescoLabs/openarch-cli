package loader

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Fingerprint identifies the embeddings of a document: the SHA-256 of
// {"model": model, "passages": passages} as JSON with sorted keys, ", " and
// ": " as separators and non-ASCII characters as they are (the form of
// Python's json.dumps(value, sort_keys=True, ensure_ascii=False)). It is stored
// as the sys_description of the embeddings document, and names the file in
// the cache. Embeddings with the same fingerprint are not made again.
func Fingerprint(model string, passages []map[string]any) (string, error) {
	list := make([]any, len(passages))
	for i, p := range passages {
		list[i] = p
	}
	var b bytes.Buffer
	if err := writePython(&b, map[string]any{"model": model, "passages": list}); err != nil {
		return "", err
	}
	h := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(h[:]), nil
}

// writePython writes v in the JSON form described at Fingerprint. Numbers
// must be json.Number, so they keep the form they were read in.
func writePython(w io.Writer, v any) error {
	switch x := v.(type) {
	case nil:
		_, err := io.WriteString(w, "null")
		return err
	case bool:
		_, err := fmt.Fprint(w, x)
		return err
	case json.Number:
		_, err := io.WriteString(w, string(x))
		return err
	case string:
		_, err := io.WriteString(w, pythonString(x))
		return err
	case []any:
		if _, err := io.WriteString(w, "["); err != nil {
			return err
		}
		for i, e := range x {
			if i > 0 {
				if _, err := io.WriteString(w, ", "); err != nil {
					return err
				}
			}
			if err := writePython(w, e); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, "]")
		return err
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys) // byte order of UTF-8 is code point order, as in Python
		if _, err := io.WriteString(w, "{"); err != nil {
			return err
		}
		for i, k := range keys {
			if i > 0 {
				if _, err := io.WriteString(w, ", "); err != nil {
					return err
				}
			}
			if _, err := io.WriteString(w, pythonString(k)+": "); err != nil {
				return err
			}
			if err := writePython(w, x[k]); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, "}")
		return err
	}
	return fmt.Errorf("cannot write %T as JSON", v)
}

// pythonString quotes s as Python's json.dumps with ensure_ascii=False: only
// the quote, the backslash and control characters are escaped.
func pythonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
