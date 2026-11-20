package verify

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// memSource is an in-memory Source.
type memSource map[string]string

func (m memSource) Read(p string) ([]byte, error) {
	s, ok := m[p]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return []byte(s), nil
}

func (m memSource) List(dir string) ([]string, error) {
	var out []string
	for p := range m {
		rest, ok := strings.CutPrefix(p, dir+"/")
		if ok && !strings.Contains(rest, "/") && strings.HasSuffix(rest, ".json") {
			out = append(out, rest)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (m memSource) Entries(dir string) ([]Entry, error) {
	prefix := ""
	if dir != "" {
		prefix = dir + "/"
	}
	seen := map[string]bool{}
	var out []Entry
	for p := range m {
		rest, ok := strings.CutPrefix(p, prefix)
		if !ok {
			continue
		}
		name, _, nested := strings.Cut(rest, "/")
		if seen[name] {
			continue
		}
		seen[name] = true
		k := KindFile
		if nested {
			k = KindDir
		}
		out = append(out, Entry{name, k})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

const itemI1 = `{"id": "I1", "type": "initiative", "parent": null, "state": "in_progress", "priority": 1, "spec": "work/specs/I1.json"}`
const itemF1 = `{"id": "F1", "type": "feature", "parent": "I1", "state": "in_progress", "priority": 1, "capability": "C1", "spec": "work/specs/F1.json"}`
const itemS1 = `{"id": "S1", "type": "slice", "parent": "F1", "state": "proposed", "priority": 1, "spec": "work/specs/S1.json"}`

const config = `{"role": "pack", "default_model": "m", "language": "es-VE"}`

func backlog(items ...string) string {
	if len(items) == 0 {
		items = []string{itemI1, itemF1, itemS1}
	}
	return backlogWith(`{"I": 2, "F": 2, "S": 2, "B": 1, "N": 1, "C": 2}`, items...)
}

func backlogWith(nextIDs string, items ...string) string {
	return "{\n  \"schema_version\": 1,\n  \"next_ids\": " + nextIDs + ",\n" +
		"  \"capabilities\": [{\"id\": \"C1\", \"name\": \"Instalar\", \"page\": \"docs/governance/C1-install.md\"}],\n" +
		"  \"items\": [\n    " + strings.Join(items, ",\n    ") + "\n  ]\n}\n"
}

func repo(extra map[string]string) memSource {
	m := memSource{"backlog.json": backlog(), "canon/config.json": config}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func lines(t *testing.T, src Source) (int, []string) {
	t.Helper()
	vs, err := Run(src)
	if err != nil {
		t.Fatalf("Run devolvió error: %v", err)
	}
	var out []string
	for _, v := range vs {
		out = append(out, v.String())
	}
	code := 0
	if len(out) > 0 {
		code = 1
	}
	return code, out
}

func mustContain(t *testing.T, line string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(line, p) {
			t.Errorf("el mensaje %q no contiene %q", line, p)
		}
	}
}

func noLibraryText(t *testing.T, ls []string) {
	t.Helper()
	for _, l := range ls {
		_, l, _ = strings.Cut(l, ": ") // the file name itself ends in ".json"
		for _, bad := range []string{"panic", "json:", "cannot unmarshal", "unexpected", "invalid character", "EOF"} {
			if strings.Contains(l, bad) {
				t.Errorf("el mensaje %q contiene texto en inglés o de la biblioteca (%q)", l, bad)
			}
		}
	}
}

func TestBaseFixtureIsGreen(t *testing.T) {
	code, ls := lines(t, repo(nil))
	if code != 0 {
		t.Fatalf("el repo base debería dar verde: %q", ls)
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	item := func(extra string) string {
		return `{"id": "S2", "type": "slice", "parent": "F1", "state": "proposed", "priority": 2, "spec": "work/specs/S2.json"` + extra + `}`
	}

	t.Run("campo inventado en el tercer elemento", func(t *testing.T) {
		b := backlog(itemI1, itemF1, strings.Replace(itemS1, `"priority": 1`, `"priority": 1, "color": "rojo"`, 1))
		code, ls := lines(t, repo(map[string]string{"backlog.json": b}))
		if code != 1 || len(ls) != 1 {
			t.Fatalf("código %d, mensajes %q", code, ls)
		}
		mustContain(t, ls[0], "backlog.json", "items[2]", "color")
		noLibraryText(t, ls)
	})

	t.Run("mayúsculas en las claves", func(t *testing.T) {
		b := backlog(itemI1, strings.Replace(itemF1, `"id"`, `"ID"`, 1), itemS1)
		code, ls := lines(t, repo(map[string]string{"backlog.json": b}))
		if code != 1 {
			t.Fatalf("código %d", code)
		}
		all := strings.Join(ls, "\n")
		mustContain(t, all, "backlog.json", "items[1]", "ID", "id")
		if !strings.Contains(all, "sobrante") || !strings.Contains(all, "falta") {
			t.Errorf("debe decir sobrante y falta: %q", all)
		}
		b2 := strings.Replace(backlog(), `"items"`, `"Items"`, 1)
		code, ls = lines(t, repo(map[string]string{"backlog.json": b2}))
		all = strings.Join(ls, "\n")
		if code != 1 {
			t.Fatalf("código %d", code)
		}
		mustContain(t, all, "Items", "items", "sobrante", "falta")
	})

	t.Run("clave repetida", func(t *testing.T) {
		dup := strings.Replace(itemS1, `"state": "proposed"`, `"state": "done", "state": "proposed"`, 1)
		code, ls := lines(t, repo(map[string]string{"backlog.json": backlog(itemI1, itemF1, dup)}))
		if code != 1 || len(ls) != 1 {
			t.Fatalf("código %d, mensajes %q", code, ls)
		}
		mustContain(t, ls[0], "backlog.json", "items[2]", "state", "repetida")
	})

	t.Run("contenido después del valor", func(t *testing.T) {
		for name, tail := range map[string]string{"objeto": "{}", "texto": "hola"} {
			code, ls := lines(t, repo(map[string]string{"backlog.json": backlog() + tail}))
			if code != 1 || len(ls) != 1 {
				t.Fatalf("%s: código %d, mensajes %q", name, code, ls)
			}
			mustContain(t, ls[0], "backlog.json", "después")
			noLibraryText(t, ls)
		}
		code, ls := lines(t, repo(map[string]string{"backlog.json": backlog() + "   \n\n"}))
		if code != 0 {
			t.Errorf("solo espacios y saltos deben dar verde: %q", ls)
		}
	})

	t.Run("tipos, null y campo ausente", func(t *testing.T) {
		cases := []struct {
			name, item, field string
		}{
			{"priority texto", strings.Replace(item(""), `"priority": 2`, `"priority": "1"`, 1), "priority"},
			{"id null", strings.Replace(item(""), `"id": "S2"`, `"id": null`, 1), "id"},
			{"sin type", strings.Replace(item(""), `"type": "slice", `, ``, 1), "type"},
		}
		for _, c := range cases {
			code, ls := lines(t, repo(map[string]string{"backlog.json": backlog(itemI1, itemF1, itemS1, c.item)}))
			if code != 1 || len(ls) != 1 {
				t.Fatalf("%s: código %d, mensajes %q", c.name, code, ls)
			}
			mustContain(t, ls[0], "backlog.json", "items[3]", c.field)
			noLibraryText(t, ls)
		}
	})

	t.Run("tres fallas en un archivo, orden y determinismo", func(t *testing.T) {
		a := strings.Replace(item(""), `"priority": 2`, `"priority": "1"`, 1)
		b := strings.Replace(strings.Replace(item(""), `"id": "S2"`, `"id": null`, 1), `S2`, `S3`, 1)
		c := strings.Replace(item(""), `"type": "slice", `, ``, 1)
		src := repo(map[string]string{"backlog.json": backlog(itemI1, itemF1, a, b, c)})
		code, ls := lines(t, src)
		if code != 1 || len(ls) != 3 {
			t.Fatalf("código %d, mensajes %q", code, ls)
		}
		mustContain(t, ls[0], "items[2]", "priority")
		mustContain(t, ls[1], "items[3]", "id")
		mustContain(t, ls[2], "items[4]", "type")
		_, again := lines(t, src)
		if strings.Join(ls, "\n") != strings.Join(again, "\n") {
			t.Errorf("la salida no es estable:\n%q\n%q", ls, again)
		}
	})

	t.Run("números", func(t *testing.T) {
		for _, v := range []string{"1.5", "-1", "1e2"} {
			b := backlog(itemI1, itemF1, strings.Replace(itemS1, `"priority": 1`, `"priority": `+v, 1))
			code, ls := lines(t, repo(map[string]string{"backlog.json": b}))
			if code != 1 || len(ls) != 1 {
				t.Fatalf("priority %s: código %d, mensajes %q", v, code, ls)
			}
			mustContain(t, ls[0], "priority", v)
		}
		for _, v := range []string{"0", `"47"`} {
			b := backlogWith(`{"I": 2, "F": 2, "S": `+v+`, "B": 1, "N": 1, "C": 2}`, itemI1, itemF1, itemS1)
			code, ls := lines(t, repo(map[string]string{"backlog.json": b}))
			if code != 1 || len(ls) != 1 {
				t.Fatalf("next_ids.S %s: código %d, mensajes %q", v, code, ls)
			}
			mustContain(t, ls[0], "next_ids", "S", v)
		}
	})

	t.Run("listas cerradas", func(t *testing.T) {
		cases := []struct{ name, item, want string }{
			{"Done", strings.Replace(itemS1, `"proposed"`, `"Done"`, 1), "to_refine, proposed, ready, in_progress, done"},
			{"finished", strings.Replace(itemS1, `"proposed"`, `"finished"`, 1), "to_refine, proposed, ready, in_progress, done"},
			{"epic", strings.Replace(itemS1, `"slice"`, `"epic"`, 1), "initiative, feature, slice, bug, need"},
		}
		for _, c := range cases {
			code, ls := lines(t, repo(map[string]string{"backlog.json": backlog(itemI1, itemF1, c.item)}))
			if code != 1 || len(ls) != 1 {
				t.Fatalf("%s: código %d, mensajes %q", c.name, code, ls)
			}
			mustContain(t, ls[0], "items[2]", c.name, c.want)
		}
	})

	t.Run("BOM y CRLF", func(t *testing.T) {
		code, ls := lines(t, repo(map[string]string{"backlog.json": "\xef\xbb\xbf" + backlog()}))
		if code != 1 || len(ls) != 1 {
			t.Fatalf("BOM: código %d, mensajes %q", code, ls)
		}
		mustContain(t, ls[0], "backlog.json", "BOM")
		noLibraryText(t, ls)
		code, ls = lines(t, repo(map[string]string{"backlog.json": strings.ReplaceAll(backlog(), "\n", "\r\n")}))
		if code != 0 {
			t.Errorf("CRLF debe dar verde: %q", ls)
		}
	})

	t.Run("archivo vacío, lista, null y ausente", func(t *testing.T) {
		for name, content := range map[string]string{"vacío": "", "lista": "[]", "null": "null"} {
			code, ls := lines(t, repo(map[string]string{"backlog.json": content}))
			if code != 1 || len(ls) == 0 {
				t.Fatalf("%s: código %d, mensajes %q", name, code, ls)
			}
			mustContain(t, ls[0], "backlog.json")
			noLibraryText(t, ls)
		}
		src := repo(nil)
		delete(src, "backlog.json")
		code, ls := lines(t, src)
		if code != 1 || len(ls) == 0 {
			t.Fatalf("ausente: código %d, mensajes %q", code, ls)
		}
		mustContain(t, ls[0], "backlog.json", "falta")
	})

	t.Run("JSON mal formado", func(t *testing.T) {
		for _, content := range []string{`{"schema_version": 1,`, `{"a" 1}`, `{"a": tru}`, `{,}`} {
			code, ls := lines(t, repo(map[string]string{"backlog.json": content}))
			if code != 1 {
				t.Fatalf("%q: código %d", content, code)
			}
			mustContain(t, ls[0], "backlog.json")
			noLibraryText(t, ls)
		}
	})

	t.Run("campo sobrante en spec, solicitud y config", func(t *testing.T) {
		spec := `{"id": "S1", "parent": "F1", "name": "n", "what": "w", "files": ["a"], "functional_test": "f", "technical_test": "no aplica", "destructive_cases": [], "color": "rojo"}`
		req := `{"id": "N2", "date": "d", "type": "error", "raw_message": "m", "color": "rojo"}`
		cfg := `{"role": "pack", "default_model": "m", "language": "es", "color": "rojo"}`
		for file, content := range map[string]string{"work/specs/S1.json": spec, "work/requests/N2.json": req, "canon/config.json": cfg} {
			code, ls := lines(t, repo(map[string]string{file: content}))
			if code != 1 || len(ls) != 1 {
				t.Fatalf("%s: código %d, mensajes %q", file, code, ls)
			}
			mustContain(t, ls[0], file, "color")
		}
	})

	t.Run("role y archivo de spec desconocido", func(t *testing.T) {
		code, ls := lines(t, repo(map[string]string{"canon/config.json": `{"role": "otro", "default_model": "m", "language": "es"}`}))
		if code != 1 {
			t.Fatalf("código %d", code)
		}
		mustContain(t, ls[0], "role", "pack, instance")
		code, ls = lines(t, repo(map[string]string{"work/specs/X1.json": `{}`}))
		if code != 1 {
			t.Fatalf("código %d", code)
		}
		mustContain(t, ls[0], "work/specs/X1.json")
	})

	t.Run("formas válidas de technical_test", func(t *testing.T) {
		slice := func(tt string) string {
			return `{"id": "S1", "parent": "F1", "name": "n", "what": "w", "files": [], "functional_test": "f", "technical_test": ` + tt + `, "destructive_cases": [{"case": "c", "expect": "e"}]}`
		}
		for _, tt := range []string{`"texto"`, `{"name": "T", "red_before_code": true}`, `{"name": "T", "red_before_code": false, "also": ["A"]}`} {
			code, ls := lines(t, repo(map[string]string{"work/specs/S1.json": slice(tt)}))
			if code != 0 {
				t.Errorf("%s debe ser válido: %q", tt, ls)
			}
		}
		code, ls := lines(t, repo(map[string]string{"work/specs/S1.json": slice(`7`)}))
		if code != 1 || len(ls) != 1 {
			t.Fatalf("technical_test numérico: código %d %q", code, ls)
		}
		mustContain(t, ls[0], "technical_test")
		bug := `{"id": "B1", "parent": "S1", "name": "n", "what": "w", "files": [], "repro": {"name": "T", "red": "r"}, "functional_test": "f", "destructive_cases": [], "platforms_note": "p"}`
		code, ls = lines(t, repo(map[string]string{"work/specs/B1.json": bug}))
		if code != 0 {
			t.Errorf("un bug sin technical_test debe ser válido: %q", ls)
		}
		slice0 := strings.Replace(slice(`"x"`), `"technical_test": "x", `, ``, 1)
		code, ls = lines(t, repo(map[string]string{"work/specs/S1.json": slice0}))
		if code != 1 {
			t.Fatalf("un slice sin technical_test debe fallar: %q", ls)
		}
		mustContain(t, ls[0], "technical_test")
	})

	t.Run("solicitud con turn y campos nulos", func(t *testing.T) {
		req := `{"id": "N3", "date": "d", "type": "need", "raw_message": "m", "turn": "T7", "scenario": null, "rounds": [], "result": null, "adr": null, "glossary": null}`
		code, ls := lines(t, repo(map[string]string{"work/requests/N3.json": req}))
		if code != 0 {
			t.Errorf("solicitud válida rechazada: %q", ls)
		}
	})
}

func TestRepoFilesDecode(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := FindRoot(wd)
	if err != nil {
		t.Fatalf("no se encontró la raíz del repo: %v", err)
	}
	src := Disk(root)
	for _, f := range []string{
		"backlog.json", "canon/config.json", "archive/I1.json", "work/requests/N1.json", "work/requests/N2.json",
		"work/specs/S2.json", "work/specs/S8.json", "work/specs/B1.json", "work/specs/B3.json", "work/specs/B6.json",
		"work/specs/S9.json", "work/specs/F2.json", "work/specs/I1.json",
	} {
		if _, err := src.Read(f); err != nil {
			t.Errorf("no se pudo leer %s: %v", f, err)
		}
	}
	vs, err := Run(src)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, v := range vs {
		t.Errorf("violación en un archivo real del repo: %s", v)
	}
	// All real spec files are seen by the lister.
	names, err := src.List("work/specs")
	if err != nil || len(names) < 40 {
		t.Errorf("List(work/specs) = %d archivos, err %v", len(names), err)
	}
}

func TestFindRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "backlog.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "tools", "internal")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := FindRoot(deep)
	if err != nil {
		t.Fatalf("FindRoot: %v", err)
	}
	if got != root {
		t.Errorf("raíz = %q, se esperaba %q", got, root)
	}
	if _, err := FindRoot(t.TempDir()); err == nil {
		t.Error("una carpeta sin backlog.json debe fallar")
	} else if !strings.Contains(err.Error(), "backlog.json") {
		t.Errorf("el error debe nombrar backlog.json: %v", err)
	}
}

func TestDiskSource(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "work", "specs", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"work/specs/S1.json", "work/specs/B1.json", "work/specs/notas.txt", "work/specs/sub/X.json"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(f)), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := Disk(root)
	names, err := src.List("work/specs")
	if err != nil || strings.Join(names, ",") != "B1.json,S1.json" {
		t.Errorf("List = %v, %v", names, err)
	}
	if names, err := src.List("archive"); err != nil || len(names) != 0 {
		t.Errorf("una carpeta ausente se lista vacía: %v, %v", names, err)
	}
	if _, err := src.Read("nada.json"); !os.IsNotExist(err) && !strings.Contains(fmt.Sprint(err), "not") {
		t.Errorf("Read de un archivo ausente debe dar fs.ErrNotExist: %v", err)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=", "GIT_CONFIG_SYSTEM=", "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestIndexSource(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no está instalado")
	}
	root := t.TempDir()
	git(t, root, "init", "-q")
	write := func(rel, content string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("backlog.json", backlog())
	write("canon/config.json", config)
	write("work/specs/S1.json", `{"staged": true}`)
	git(t, root, "add", ".")
	// The working tree now differs from the index.
	write("backlog.json", `{"worktree": true}`)
	write("work/specs/S2.json", `{"untracked": true}`)

	// Inherited git variables must not redirect the lookup.
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "otro.git"))
	t.Setenv("GIT_WORK_TREE", t.TempDir())

	src, err := Index(root)
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	data, err := src.Read("backlog.json")
	if err != nil || string(data) != backlog() {
		t.Errorf("debe leer el índice, no el disco: %q, %v", data, err)
	}
	names, err := src.List("work/specs")
	if err != nil || strings.Join(names, ",") != "S1.json" {
		t.Errorf("List del índice = %v, %v", names, err)
	}
	if _, err := src.Read("work/specs/S2.json"); err == nil {
		t.Error("un archivo sin añadir al índice no debe leerse")
	} else if !os.IsNotExist(err) && !errorsIsNotExist(err) {
		t.Errorf("debe dar fs.ErrNotExist: %v", err)
	}
	vs, err := Run(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		if v.File == "backlog.json" {
			t.Errorf("el backlog del índice es válido pero se reportó: %s", v)
		}
	}
}

func errorsIsNotExist(err error) bool {
	return err != nil && strings.Contains(err.Error(), "does not exist") || os.IsNotExist(err)
}

func TestIndexSourceHonorsGitIndexFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no está instalado")
	}
	root := t.TempDir()
	git(t, root, "init", "-q")
	if err := os.WriteFile(filepath.Join(root, "backlog.json"), []byte(`{"principal": 1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", ".")
	alt := filepath.Join(t.TempDir(), "indice-alterno")
	t.Setenv("GIT_INDEX_FILE", alt)
	if err := os.WriteFile(filepath.Join(root, "backlog.json"), []byte(`{"alterno": 1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", ".") // writes into the alternate index
	src, err := Index(root)
	if err != nil {
		t.Fatal(err)
	}
	data, err := src.Read("backlog.json")
	if err != nil || string(data) != `{"alterno": 1}` {
		t.Errorf("debe leer el índice de GIT_INDEX_FILE: %q, %v", data, err)
	}
}

func TestIndexOutsideRepoFails(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no está instalado")
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(t.TempDir()))
	src, err := Index(t.TempDir())
	if err == nil {
		_, err = src.List("work/specs")
	}
	if err == nil {
		t.Error("fuera de un repo git, leer el índice debe fallar")
	}
}
