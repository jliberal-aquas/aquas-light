package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// FileName is the file name of the schema called name.
func FileName(name string) string { return name + ".schema.json" }

// Generate returns the JSON Schema (draft 2020-12) of the struct type t,
// derived from its fields and `json` and `aq` tags. The output is
// deterministic: object keys are sorted and lists keep field order.
func Generate(t reflect.Type) ([]byte, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("el esquema parte de un struct, no de %s", t)
	}
	node, err := typeSchema(t, field{})
	if err != nil {
		return nil, err
	}
	node["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	node["title"] = t.Name()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(node); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func typeSchema(t reflect.Type, f field) (map[string]any, error) {
	if t.Kind() == reflect.Pointer {
		return typeSchema(t.Elem(), f)
	}
	var node map[string]any
	switch {
	case t.Kind() == reflect.Interface:
		return map[string]any{}, nil
	case isUnion(t):
		var alts []any
		for i := 0; i < t.NumField(); i++ {
			alt, err := typeSchema(t.Field(i).Type, field{})
			if err != nil {
				return nil, err
			}
			alts = append(alts, alt)
		}
		return map[string]any{"oneOf": alts}, nil
	}
	switch t.Kind() {
	case reflect.String:
		node = map[string]any{"type": "string"}
		if len(f.enum) > 0 {
			node["enum"] = enumValues(f)
		}
	case reflect.Int, reflect.Int64:
		node = map[string]any{"type": "integer"}
		if f.min != nil {
			node["minimum"] = *f.min
		}
	case reflect.Bool:
		node = map[string]any{"type": "boolean"}
	case reflect.Slice:
		items, err := typeSchema(t.Elem(), field{enum: f.enum})
		if err != nil {
			return nil, err
		}
		node = map[string]any{"type": "array", "items": items}
	case reflect.Struct:
		info := infoOf(t)
		props := map[string]any{}
		var required []string
		for _, fi := range info.fields {
			sub, err := typeSchema(t.Field(fi.index).Type, fi)
			if err != nil {
				return nil, err
			}
			props[fi.name] = sub
			if !fi.optional {
				required = append(required, fi.name)
			}
		}
		node = map[string]any{"type": "object", "additionalProperties": false, "properties": props}
		if len(required) > 0 {
			node["required"] = required
		}
	default:
		return nil, fmt.Errorf("tipo no soportado %s", t)
	}
	if f.nullable {
		node["type"] = []string{node["type"].(string), "null"}
		if enum, ok := node["enum"].([]any); ok {
			node["enum"] = append(enum, nil)
		}
	}
	return node, nil
}

func enumValues(f field) []any {
	out := make([]any, len(f.enum))
	for i, v := range f.enum {
		out[i] = v
	}
	return out
}

// Write generates every registered schema into dir.
func Write(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, e := range Registry() {
		data, err := Generate(e.Type)
		if err != nil {
			return fmt.Errorf("%s: %w", e.Name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, FileName(e.Name)), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Stale lists the schema files of dir that are missing or differ from what
// the Go types generate. Line endings are compared as LF.
func Stale(dir string) ([]string, error) {
	var stale []string
	for _, e := range Registry() {
		want, err := Generate(e.Type)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name, err)
		}
		got, err := os.ReadFile(filepath.Join(dir, FileName(e.Name)))
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		got = []byte(strings.ReplaceAll(string(got), "\r\n", "\n"))
		if err != nil || !bytes.Equal(got, want) {
			stale = append(stale, FileName(e.Name))
		}
	}
	return stale, nil
}
