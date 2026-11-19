package loader

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// schemaFile is the content model fragment that a load needs.
type schemaFile struct {
	Schemas    map[string]schemaDef `json:"schemas"`
	MixinTypes map[string]mixinDef  `json:"mixinTypes"`
}

type schemaDef struct {
	Prefix string              `json:"prefix"`
	Fields map[string]fieldDef `json:"fields"`
}

type fieldDef struct {
	Type string `json:"type"`
}

type mixinDef struct {
	Schemas []string `json:"schemas"`
}

// EnsureModel adds to the content model what the fragment has and the model
// misses: schemas, fields and mixin types. A field or mixin type in the model
// can never change, so a fragment that differs from the model is an error, and
// nothing is changed. The patch is first checked with validateOnly. It returns
// the number of operations applied.
func EnsureModel(ctx context.Context, c Client, fragment []byte) (int, error) {
	var want schemaFile
	if err := json.Unmarshal(fragment, &want); err != nil {
		return 0, output.Usage("the schema file is not valid: "+err.Error(), `Use {"schemas": {...}, "mixinTypes": {...}}, as in GET /api/repository/model.`)
	}
	data, err := c.Model(ctx)
	if err != nil {
		return 0, err
	}
	var have schemaFile
	if err := json.Unmarshal(data, &have); err != nil {
		return 0, fmt.Errorf("the content model from the server: %w", err)
	}
	var patch []map[string]any
	add := func(path string, value any) {
		patch = append(patch, map[string]any{"op": "add", "path": path, "value": value})
	}
	for _, name := range sortedKeys(want.Schemas) {
		w := want.Schemas[name]
		h, ok := have.Schemas[name]
		if !ok {
			add("/schemas/"+pointer(name), w)
			continue
		}
		if h.Prefix != w.Prefix {
			return 0, output.Usage(fmt.Sprintf("schema %s has prefix %q in the repository, not %q", name, h.Prefix, w.Prefix), "A schema cannot change. Fix the schema file.")
		}
		for _, field := range sortedKeys(w.Fields) {
			f, ok := h.Fields[field]
			switch {
			case !ok:
				add("/schemas/"+pointer(name)+"/fields/"+pointer(field), w.Fields[field])
			case f.Type != w.Fields[field].Type:
				return 0, output.Usage(fmt.Sprintf("field %s is of type %s in the repository, not %s", field, f.Type, w.Fields[field].Type), "A field cannot change. Fix the schema file, or use a new field.")
			}
		}
	}
	for _, name := range sortedKeys(want.MixinTypes) {
		h, ok := have.MixinTypes[name]
		switch {
		case !ok:
			add("/mixinTypes/"+pointer(name), want.MixinTypes[name])
		case !reflect.DeepEqual(h.Schemas, want.MixinTypes[name].Schemas):
			return 0, output.Usage(fmt.Sprintf("mixin type %s has schemas %v in the repository, not %v", name, h.Schemas, want.MixinTypes[name].Schemas), "A mixin type cannot change. Fix the schema file.")
		}
	}
	if len(patch) == 0 {
		return 0, nil
	}
	body := mustJSON(patch)
	if err := c.PatchModel(ctx, body, true); err != nil {
		return 0, err
	}
	if err := c.PatchModel(ctx, body, false); err != nil {
		return 0, err
	}
	return len(patch), nil
}

// pointer escapes a name for a JSON Pointer (RFC 6901).
func pointer(s string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(s)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
