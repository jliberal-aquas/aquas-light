package install

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func fakeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	w := func(rel string, data []byte) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("canon/a.md", []byte("uno\r\ndos\n"))
	w("canon/sub/b.json", []byte("{}\n"))
	w("canon/manifest.json", []byte("viejo"))
	w("tools/go.mod", []byte("module x\n"))
	w("tools/data.bin", []byte{0x00, 0x0D, 0x0A, 0xFF, 0x0D, 0x0A})
	w("tools/bin/aquas", []byte("compilado"))
	w("templates/backlog.json", []byte("{\"schema_version\": 1, \"next_ids\": {\"S\": 1}, \"capabilities\": [], \"items\": []}\n"))
	w("templates/README.md", []byte("# Hola\n"))
	w("templates/gitignore", []byte("bin/\n"))
	w("templates/gitattributes", []byte("* text=auto eol=lf\n"))
	w("templates/plantilla.bin", []byte{0x00, 0x0D, 0x0A, 0xFF, 0x0D, 0x0A})
	return root
}

func readManifest(t *testing.T, dest string) (Manifest, []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dest, "canon", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m, raw
}

func TestCopyBinaryIdentical(t *testing.T) {
	root := fakeRoot(t)
	dest := filepath.Join(t.TempDir(), "d")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Copy(root, dest, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"tools/data.bin", "canon/a.md", "templates/plantilla.bin"} {
		want, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		target := rel
		if strings.HasPrefix(rel, "templates/") {
			target = strings.TrimPrefix(rel, "templates/")
		}
		got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(target)))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s no es idéntico byte a byte (err=%v)", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "tools", "bin")); err == nil {
		t.Error("tools/bin no debe copiarse")
	}
}

func TestCopyManifestDeterministicAndConsistent(t *testing.T) {
	root := fakeRoot(t)
	var raws [][]byte
	var dests []string
	for i := 0; i < 2; i++ {
		dest := filepath.Join(t.TempDir(), "instalacion")
		if err := os.Mkdir(dest, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := Copy(root, dest, "0.1.0"); err != nil {
			t.Fatal(err)
		}
		_, raw := readManifest(t, dest)
		raws = append(raws, raw)
		dests = append(dests, dest)
	}
	if !bytes.Equal(raws[0], raws[1]) {
		t.Fatal("los manifiestos de dos instalaciones difieren")
	}
	m, _ := readManifest(t, dests[0])
	var keys []string
	for k := range m.Files {
		keys = append(keys, k)
		if strings.Contains(k, `\`) || filepath.IsAbs(k) || strings.HasPrefix(k, "/") || strings.Contains(k, "instalacion") || strings.Contains(k, dests[0]) {
			t.Errorf("ruta inválida en el manifiesto: %q", k)
		}
	}
	if strings.Contains(string(raws[0]), dests[0]) {
		t.Error("el manifiesto contiene la ruta temporal")
	}
	// Order on disk: keys appear sorted in the raw file.
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	last := -1
	for _, k := range sorted {
		i := strings.Index(string(raws[0]), `"`+k+`"`)
		if i < last {
			t.Errorf("rutas fuera de orden en el archivo: %s", k)
		}
		last = i
	}

	// Cross-check canon/ with the manifest.
	seen := map[string]int{}
	err := filepath.WalkDir(filepath.Join(dests[0], "canon"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dests[0], p)
		seen[filepath.ToSlash(rel)]++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for k := range m.Files {
		if _, ok := seen[k]; !ok && !strings.HasPrefix(k, "tools/") && !strings.HasPrefix(k, "schemas/") {
			t.Errorf("entrada sin archivo: %s", k)
		}
		if _, err := os.Stat(filepath.Join(dests[0], filepath.FromSlash(k))); err != nil {
			t.Errorf("entrada que apunta a un archivo inexistente: %s", k)
		}
	}
	for k := range seen {
		if k == "canon/manifest.json" {
			if _, ok := m.Files[k]; ok {
				t.Error("manifest.json se lista a sí mismo")
			}
			continue
		}
		if _, ok := m.Files[k]; !ok {
			t.Errorf("archivo sin entrada: %s", k)
		}
	}
	for k := range m.Files {
		if strings.HasPrefix(k, "templates/") {
			t.Errorf("las plantillas no entran al manifiesto: %s", k)
		}
	}
	if _, ok := m.Files["tools/bin/aquas"]; ok {
		t.Error("tools/bin no debe estar en el manifiesto")
	}
}

func TestCopyStarterFiles(t *testing.T) {
	dest := t.TempDir()
	if err := Copy(fakeRoot(t), dest, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"work", "archive", "product", "docs"} {
		if _, err := os.Stat(filepath.Join(dest, d, ".gitkeep")); err != nil {
			t.Errorf("falta %s/.gitkeep", d)
		}
	}
	ga, _ := os.ReadFile(filepath.Join(dest, ".gitattributes"))
	if string(ga) != "* text=auto eol=lf\n" {
		t.Errorf(".gitattributes = %q", ga)
	}
	if _, err := os.Stat(filepath.Join(dest, "templates")); err == nil {
		t.Error("templates/ no debe copiarse como carpeta")
	}
	if gi, _ := os.ReadFile(filepath.Join(dest, ".gitignore")); string(gi) != "bin/\n" {
		t.Errorf(".gitignore = %q", gi)
	}
	var b struct {
		V   int            `json:"schema_version"`
		N   map[string]int `json:"next_ids"`
		C   []any          `json:"capabilities"`
		Its []any          `json:"items"`
	}
	raw, _ := os.ReadFile(filepath.Join(dest, "backlog.json"))
	if err := json.Unmarshal(raw, &b); err != nil || b.V != 1 || len(b.N) == 0 || len(b.C) != 0 || len(b.Its) != 0 {
		t.Errorf("backlog.json inválido: %s (%v)", raw, err)
	}
	for _, n := range b.N {
		if n != 1 {
			t.Errorf("next_ids debe estar en 1: %v", b.N)
		}
	}
	for _, f := range []string{"README.md", ".gitignore", "backlog.json"} {
		data, _ := os.ReadFile(filepath.Join(dest, f))
		if len(data) == 0 || bytes.Contains(data, []byte("\r")) {
			t.Errorf("%s vacío o con CR", f)
		}
	}
}

func TestFindPackageRoot(t *testing.T) {
	root := fakeRoot(t)
	deep := filepath.Join(root, "tools", "x", "y")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := FindPackageRoot(deep)
	if err != nil {
		t.Fatal(err)
	}
	if a, _ := filepath.EvalSymlinks(got); a != func() string { r, _ := filepath.EvalSymlinks(root); return r }() {
		t.Errorf("raíz = %s, se esperaba %s", got, root)
	}
	if _, err := FindPackageRoot(t.TempDir()); err == nil {
		t.Error("se esperaba error fuera del paquete")
	}
}
