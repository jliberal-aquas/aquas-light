package schema

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func repoSchemas() string { return filepath.Join("..", "..", "..", "schemas") }

func TestSchemasMatchTypes(t *testing.T) {
	stale, err := Stale(repoSchemas())
	if err != nil {
		t.Fatalf("Stale: %v", err)
	}
	for _, name := range stale {
		t.Errorf("esquema desactualizado o ausente: schemas/%s (corre `go run ./internal/schema/gen ../schemas` desde tools/)", name)
	}
	for _, e := range Registry() {
		data, err := os.ReadFile(filepath.Join(repoSchemas(), FileName(e.Name)))
		if err != nil {
			t.Errorf("falta schemas/%s: %v", FileName(e.Name), err)
			continue
		}
		var v map[string]any
		if err := json.Unmarshal(data, &v); err != nil {
			t.Errorf("schemas/%s no es JSON: %v", FileName(e.Name), err)
		}
		if bytes.Contains(data, []byte("\r")) {
			t.Errorf("schemas/%s debe usar LF", FileName(e.Name))
		}
	}
}

func TestSchemaStaleIsDetectedByName(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if stale, err := Stale(dir); err != nil || len(stale) != 0 {
		t.Fatalf("recién generados no deben estar desactualizados: %v, %v", stale, err)
	}
	p := filepath.Join(dir, FileName("config"))
	if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale, err := Stale(dir)
	if err != nil || len(stale) != 1 || stale[0] != FileName("config") {
		t.Errorf("debe nombrar config.schema.json: %v, %v", stale, err)
	}
	if err := os.Remove(filepath.Join(dir, FileName("backlog"))); err != nil {
		t.Fatal(err)
	}
	stale, _ = Stale(dir)
	if len(stale) != 2 {
		t.Errorf("un esquema ausente también cuenta: %v", stale)
	}
}

func TestGenerateChangesWithTypes(t *testing.T) {
	base, err := Generate(reflect.TypeFor[Config]())
	if err != nil || len(base) == 0 {
		t.Fatalf("Generate(Config) = %q, %v", base, err)
	}
	extended := reflect.StructOf([]reflect.StructField{
		{Name: "Role", Type: reflect.TypeFor[string](), Tag: `json:"role" aq:"enum=pack|instance"`},
		{Name: "DefaultModel", Type: reflect.TypeFor[string](), Tag: `json:"default_model"`},
		{Name: "Language", Type: reflect.TypeFor[string](), Tag: `json:"language"`},
		{Name: "Color", Type: reflect.TypeFor[string](), Tag: `json:"color"`},
	})
	other, err := Generate(extended)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(base, other) || !strings.Contains(string(other), `"color"`) {
		t.Errorf("agregar un campo debe cambiar el esquema:\n%s", other)
	}
	again, _ := Generate(reflect.TypeFor[Config]())
	if !bytes.Equal(base, again) {
		t.Error("Generate no es determinista")
	}
}

func TestGeneratedSchemaShape(t *testing.T) {
	data, err := Generate(reflect.TypeFor[BacklogItem]())
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("no es JSON: %v\n%s", err, data)
	}
	if s["additionalProperties"] != false || s["type"] != "object" {
		t.Errorf("debe cerrar el objeto: %v", s)
	}
	req, _ := s["required"].([]any)
	got := map[string]bool{}
	for _, r := range req {
		got[r.(string)] = true
	}
	for _, k := range []string{"id", "type", "parent", "state", "priority", "spec"} {
		if !got[k] {
			t.Errorf("%q debe ser obligatorio", k)
		}
	}
	for _, k := range []string{"capability", "source", "spec_sha256", "supersedes"} {
		if got[k] {
			t.Errorf("%q debe ser opcional", k)
		}
	}
	props := s["properties"].(map[string]any)
	state := props["state"].(map[string]any)
	if enum, _ := state["enum"].([]any); len(enum) != 5 {
		t.Errorf("state debe listar sus cinco valores: %v", state)
	}
	if parent := props["parent"].(map[string]any); !strings.Contains(string(mustJSON(parent["type"])), "null") {
		t.Errorf("parent admite null: %v", parent)
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
