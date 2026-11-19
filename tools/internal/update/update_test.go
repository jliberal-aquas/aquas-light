package update

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jliberal-aquas/aquas-light/tools/internal/install"
)

func write(t *testing.T, root, rel string, data string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func newPackage(t *testing.T, v string) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "canon/method.md", "metodo "+v+"\n")
	write(t, root, "canon/other.md", "otro "+v+"\n")
	write(t, root, "canon/manifest.json", "viejo")
	write(t, root, "tools/go.mod", "module x\n")
	write(t, root, "tools/main.go", "package main // "+v+"\n")
	write(t, root, "templates/README.md", "# Hola\n")
	return root
}

func newInstance(t *testing.T, pkg string) string {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "inst")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := install.Copy(pkg, dest, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	return dest
}

func TestUpdateStopsOnLocalEdit(t *testing.T) {
	v1, v2 := newPackage(t, "v1"), newPackage(t, "v2")
	inst := newInstance(t, v1)
	write(t, inst, "canon/method.md", "editado a mano\n")

	res, err := Update(v2, inst, "0.2.0", Takes{})
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("se esperaba ConflictError, llegó %v", err)
	}
	if len(ce.Paths) != 1 || ce.Paths[0] != "canon/method.md" {
		t.Fatalf("rutas en conflicto = %v", ce.Paths)
	}
	if len(res.Changed) != 0 {
		t.Fatalf("no debía cambiar nada: %v", res.Changed)
	}
	if got := read(t, inst, "canon/method.md"); got != "editado a mano\n" {
		t.Fatalf("method.md cambió: %q", got)
	}
	if got := read(t, inst, "canon/other.md"); got != "otro v1\n" {
		t.Fatalf("other.md cambió pese al conflicto: %q", got)
	}
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		rel, _ := filepath.Rel(root, p)
		out[filepath.ToSlash(rel)] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func equalSnap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

func TestUpdateCRLFOnlyIsNotConflict(t *testing.T) {
	v1, v2 := newPackage(t, "v1"), newPackage(t, "v2")
	inst := newInstance(t, v1)
	write(t, inst, "canon/method.md", "metodo v1\r\n")
	res, err := Update(v2, inst, "0.2.0", Takes{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("rutas en conflicto: %v", res.Conflicts)
	}
	if got := read(t, inst, "canon/method.md"); got != "metodo v2\n" {
		t.Fatalf("no se reemplazó: %q", got)
	}
}

func TestUpdateConflictChangesNothing(t *testing.T) {
	v1, v2 := newPackage(t, "v1"), newPackage(t, "v2")
	inst := newInstance(t, v1)
	write(t, inst, "canon/method.md", "editado\n")
	before := snapshot(t, inst)
	_, err := Update(v2, inst, "0.2.0", Takes{})
	if err == nil || !strings.Contains(err.Error(), "canon/method.md") {
		t.Fatalf("error = %v", err)
	}
	if !equalSnap(before, snapshot(t, inst)) {
		t.Fatal("la instancia cambió")
	}
}

func TestUpdateInvalidTakeChangesNothing(t *testing.T) {
	v1, v2 := newPackage(t, "v1"), newPackage(t, "v2")
	inst := newInstance(t, v1)
	write(t, inst, "canon/method.md", "editado\n")
	before := snapshot(t, inst)
	_, err := Update(v2, inst, "0.2.0", Takes{Default: "mine"})
	if err == nil || !strings.Contains(err.Error(), "ours") || !strings.Contains(err.Error(), "theirs") {
		t.Fatalf("error = %v", err)
	}
	if !equalSnap(before, snapshot(t, inst)) {
		t.Fatal("la instancia cambió")
	}
}

func TestUpdateNeverTouchesProductAndReplacesGovernance(t *testing.T) {
	v1, v2 := newPackage(t, "v1"), newPackage(t, "v2")
	write(t, v1, "docs/governance/x.md", "gob v1\n")
	inst := newInstance(t, v1)
	// install.Copy does not copy docs/governance; place it by hand.
	write(t, inst, "docs/governance/x.md", "gob v1\n")
	keep := map[string]string{
		"backlog.json": "{\"mio\":1}\n", "work/x.json": "{\"mio\":2}\n",
		"docs/product/x.md": "mio3\n", "product/x.txt": "mio4\n",
	}
	for k, v := range keep {
		write(t, inst, k, v)
		write(t, v2, k, "del paquete "+k+"\n")
	}
	write(t, v2, "docs/governance/x.md", "gob v2\n")
	if _, err := Update(v2, inst, "0.2.0", Takes{}); err != nil {
		t.Fatal(err)
	}
	for k, v := range keep {
		if got := read(t, inst, k); got != v {
			t.Fatalf("%s cambió: %q", k, got)
		}
	}
	if got := read(t, inst, "docs/governance/x.md"); got != "gob v2\n" {
		t.Fatalf("gobernanza = %q", got)
	}
}

func TestUpdateTwiceIsNoop(t *testing.T) {
	v1, v2 := newPackage(t, "v1"), newPackage(t, "v2")
	inst := newInstance(t, v1)
	if _, err := Update(v2, inst, "0.2.0", Takes{}); err != nil {
		t.Fatal(err)
	}
	res, err := Update(v2, inst, "0.2.0", Takes{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Changed) != 0 || len(res.Conflicts) != 0 {
		t.Fatalf("segunda corrida: %+v", res)
	}
}

func TestUpdateOursKeepsAndReappears(t *testing.T) {
	v1, v2 := newPackage(t, "v1"), newPackage(t, "v2")
	inst := newInstance(t, v1)
	write(t, inst, "canon/method.md", "editado\n")
	if _, err := Update(v2, inst, "0.2.0", Takes{Files: map[string]string{"canon/method.md": Ours}}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, inst, "canon/method.md"); got != "editado\n" {
		t.Fatalf("ours no conservó: %q", got)
	}
	res, err := Update(v2, inst, "0.2.0", Takes{})
	var ce *ConflictError
	if !errors.As(err, &ce) || len(res.Changed) != 0 {
		t.Fatalf("debía reaparecer el conflicto: %v", err)
	}
	if _, err := Update(v2, inst, "0.2.0", Takes{Default: Theirs}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, inst, "canon/method.md"); got != "metodo v2\n" {
		t.Fatalf("theirs: %q", got)
	}
}

func TestUpdateDeletesDroppedAndSeals(t *testing.T) {
	v1, v2 := newPackage(t, "v1"), newPackage(t, "v2")
	inst := newInstance(t, v1)
	if err := os.Remove(filepath.Join(v2, "tools", "main.go")); err != nil {
		t.Fatal(err)
	}
	write(t, inst, "tools/bin/aquas", "binario")
	write(t, inst, "work/seals.json", `{"docs/governance/a.md":"viejo","work/otro":"mio"}`)
	write(t, v2, "work/seals.json", `{"docs/governance/a.md":"nuevo","work/otro":"paquete"}`)
	if _, err := Update(v2, inst, "0.2.0", Takes{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(inst, "tools", "main.go")); err == nil {
		t.Fatal("main.go debía borrarse")
	}
	if read(t, inst, "tools/bin/aquas") != "binario" {
		t.Fatal("tools/bin debía conservarse")
	}
	s := read(t, inst, "work/seals.json")
	if !strings.Contains(s, "nuevo") || !strings.Contains(s, "mio") || strings.Contains(s, "paquete") {
		t.Fatalf("seals = %s", s)
	}
}

func TestUpdateDoesNotCreateSeals(t *testing.T) {
	v1, v2 := newPackage(t, "v1"), newPackage(t, "v2")
	inst := newInstance(t, v1)
	write(t, v2, "work/seals.json", `{}`)
	if _, err := Update(v2, inst, "0.2.0", Takes{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(inst, "work", "seals.json")); err == nil {
		t.Fatal("no debía crearse")
	}
}

func manifestVersion(t *testing.T, inst string) (string, bool) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(read(t, inst, "canon/manifest.json")), &m); err != nil {
		t.Fatal(err)
	}
	v, ok := m["version"].(string)
	return v, ok
}

func TestManifestRecordsVersion(t *testing.T) {
	v1, v2 := newPackage(t, "v1"), newPackage(t, "v2")
	inst := newInstance(t, v1)
	if v, ok := manifestVersion(t, inst); !ok || v != "0.1.0" {
		t.Fatalf("version tras instalar = %q (presente %v), want 0.1.0", v, ok)
	}
	if _, err := Update(v2, inst, "0.2.0", Takes{}); err != nil {
		t.Fatal(err)
	}
	if v, ok := manifestVersion(t, inst); !ok || v != "0.2.0" {
		t.Fatalf("version tras actualizar = %q (presente %v), want 0.2.0", v, ok)
	}
}

func TestManifestWithoutVersionIsReadAndFilled(t *testing.T) {
	v1, v2 := newPackage(t, "v1"), newPackage(t, "v2")
	inst := newInstance(t, v1)
	var m map[string]any
	if err := json.Unmarshal([]byte(read(t, inst, "canon/manifest.json")), &m); err != nil {
		t.Fatal(err)
	}
	delete(m, "version")
	old, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	write(t, inst, "canon/manifest.json", string(old)+"\n")
	if _, err := Update(v2, inst, "0.2.0", Takes{}); err != nil {
		t.Fatalf("un manifiesto sin version debe leerse: %v", err)
	}
	if v, ok := manifestVersion(t, inst); !ok || v != "0.2.0" {
		t.Fatalf("version = %q (presente %v), want 0.2.0", v, ok)
	}
}

func TestManifestVersionUnchangedOnConflict(t *testing.T) {
	v1, v2 := newPackage(t, "v1"), newPackage(t, "v2")
	inst := newInstance(t, v1)
	write(t, inst, "canon/method.md", "editado\n")
	if _, err := Update(v2, inst, "0.2.0", Takes{}); err == nil {
		t.Fatal("se esperaba un error por el conflicto")
	}
	if v, ok := manifestVersion(t, inst); !ok || v != "0.1.0" {
		t.Fatalf("version = %q (presente %v), debe seguir en 0.1.0", v, ok)
	}
}

// treeSnapshot records every file, folder and link of root without following
// links: file content, "dir", or "link -> target".
func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		switch {
		case d.Type()&os.ModeSymlink != 0:
			target, lerr := os.Readlink(p)
			out[rel] = "link -> " + target
			return lerr
		case d.IsDir():
			out[rel] = "dir"
		default:
			b, rerr := os.ReadFile(p)
			out[rel] = string(b)
			return rerr
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUpdateRefusesSymlinks(t *testing.T) {
	// replaceWithLink swaps inst/rel (file or folder) for a link to target.
	replaceWithLink := func(t *testing.T, inst, rel, target string) {
		t.Helper()
		link := filepath.Join(inst, filepath.FromSlash(rel))
		if err := os.RemoveAll(link); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("no se pueden crear enlaces simbólicos aquí: %v", err)
		}
	}

	cases := []struct {
		name    string
		rel     string // path of the link inside the instance
		isDir   bool
		prepare func(t *testing.T, v2, inst string)
	}{
		{name: "archivo del canon enlazado", rel: "canon/method.md"},
		{name: "carpeta tope enlazada", rel: "templates", isDir: true,
			prepare: func(t *testing.T, v2, _ string) { write(t, v2, "templates/nuevo.md", "nuevo\n") }},
		{name: "work/seals.json enlazado", rel: "work/seals.json",
			prepare: func(t *testing.T, _, inst string) { write(t, inst, "work/seals.json", "{}\n") }},
		{name: "subcarpeta de tools enlazada", rel: "tools/sub", isDir: true,
			prepare: func(t *testing.T, v2, inst string) {
				write(t, inst, "tools/sub/a.go", "package sub\n")
				write(t, v2, "tools/sub/a.go", "package sub // v2\n")
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v1, v2 := newPackage(t, "v1"), newPackage(t, "v2")
			inst := newInstance(t, v1)
			if c.prepare != nil {
				c.prepare(t, v2, inst)
			}
			external := t.TempDir()
			target := filepath.Join(external, "externo")
			if c.isDir {
				target = external
				write(t, external, "centinela.txt", "intacto\n")
			} else if err := os.WriteFile(target, []byte("contenido externo\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			replaceWithLink(t, inst, c.rel, target)
			instBefore, extBefore := treeSnapshot(t, inst), treeSnapshot(t, external)

			res, err := Update(v2, inst, "0.2.0", Takes{})
			if err == nil {
				t.Error("se esperaba un error por el enlace simbólico")
			} else if !strings.Contains(err.Error(), c.rel) {
				t.Errorf("el error no nombra %s: %v", c.rel, err)
			}
			if len(res.Changed) != 0 {
				t.Errorf("no debía cambiar nada: %v", res.Changed)
			}
			if after := treeSnapshot(t, external); !equalSnap(extBefore, after) {
				t.Errorf("la ubicación externa cambió: antes %v, después %v", extBefore, after)
			}
			if after := treeSnapshot(t, inst); !equalSnap(instBefore, after) {
				t.Errorf("la instancia cambió: antes %v, después %v", instBefore, after)
			}
		})
	}
}

func TestUpdateValidatesBeforeWriting(t *testing.T) {
	cases := []struct {
		name    string
		rel     string // path the error must name
		runs    int
		prepare func(t *testing.T, v2, inst string)
	}{
		{name: "carpeta en lugar de templates/README.md", rel: "templates/README.md", runs: 10,
			prepare: func(t *testing.T, v2, inst string) {
				write(t, v2, "tools/extra.go", "package main // nuevo\n")
				write(t, v2, "docs/governance/g.md", "gobernanza\n")
				if err := os.RemoveAll(filepath.Join(inst, "templates", "README.md")); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(inst, "templates", "README.md"), 0o755); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "work/seals.json inválido", rel: "work/seals.json", runs: 1,
			prepare: func(t *testing.T, v2, inst string) {
				write(t, inst, "work/seals.json", "{no es json")
				write(t, v2, "work/seals.json", `{"docs/governance/g.md":"x"}`)
				write(t, v2, "docs/governance/g.md", "gobernanza\n")
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v1, v2 := newPackage(t, "v1"), newPackage(t, "v2")
			inst := newInstance(t, v1)
			c.prepare(t, v2, inst)
			before := treeSnapshot(t, inst)
			for i := 0; i < c.runs; i++ {
				res, err := Update(v2, inst, "0.2.0", Takes{})
				if err == nil {
					t.Fatalf("corrida %d: se esperaba un error", i)
				}
				var pe *PartialError
				if errors.As(err, &pe) {
					t.Errorf("corrida %d: el error debía ser de validación, no PartialError: %v", i, err)
				}
				if !strings.Contains(err.Error(), c.rel) {
					t.Errorf("corrida %d: el error no nombra %s: %v", i, c.rel, err)
				}
				if len(res.Changed) != 0 {
					t.Errorf("corrida %d: no debía cambiar nada: %v", i, res.Changed)
				}
				if after := treeSnapshot(t, inst); !equalSnap(before, after) {
					t.Errorf("corrida %d: la instancia cambió: antes %v, después %v", i, before, after)
				}
			}
		})
	}
}

func TestUpdateWriteFailureIsPartialError(t *testing.T) {
	v1, v2 := newPackage(t, "v1"), newPackage(t, "v2")
	inst := newInstance(t, v1)
	orig := writeFile
	t.Cleanup(func() { writeFile = orig })
	calls := 0
	writeFile = func(dst string, data []byte, perm os.FileMode) error {
		calls++
		if calls == 2 {
			return errors.New("disco lleno")
		}
		return orig(dst, data, perm)
	}
	_, err := Update(v2, inst, "0.2.0", Takes{})
	var pe *PartialError
	if !errors.As(err, &pe) {
		t.Fatalf("se esperaba PartialError, llegó %v", err)
	}
	if !strings.Contains(err.Error(), "disco lleno") {
		t.Errorf("el error pierde la causa: %v", err)
	}
}
