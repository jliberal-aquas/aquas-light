package verify

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jliberal-aquas/aquas-light/tools/internal/hashing"
)

const bigNext = `{"I": 100, "F": 100, "S": 100, "B": 100, "N": 100, "C": 100}`

// it builds one backlog item. An empty parent means null.
func it(id, typ, parent, state string, extra ...string) string {
	p := "null"
	if parent != "" {
		p = fmt.Sprintf("%q", parent)
	}
	spec := fmt.Sprintf("%q", "work/specs/"+id+".json")
	if typ == "need" {
		spec = "null"
	}
	s := fmt.Sprintf(`{"id": %q, "type": %q, "parent": %s, "state": %q, "priority": 1, "spec": %s`, id, typ, p, state, spec)
	if typ == "feature" {
		s += `, "capability": "C1"`
	}
	for _, e := range extra {
		s += ", " + e
	}
	return s + "}"
}

func specText(id string) string {
	if strings.HasPrefix(id, "B") {
		return "{\n  \"id\": \"" + id + "\",\n  \"parent\": \"S1\",\n  \"name\": \"n\",\n  \"what\": \"w\",\n  \"files\": [],\n  \"repro\": {\"name\": \"T\", \"red\": \"r\"},\n  \"functional_test\": \"f\",\n  \"destructive_cases\": []\n}\n"
	}
	return "{\n  \"id\": \"" + id + "\",\n  \"parent\": \"F1\",\n  \"name\": \"n\",\n  \"what\": \"w\",\n  \"files\": [],\n  \"functional_test\": \"f\",\n  \"technical_test\": \"t\",\n  \"destructive_cases\": []\n}\n"
}

// done registers the spec file of a done slice or bug in m and returns its
// backlog entry with the right fingerprint.
func done(m memSource, id, typ, parent string) string {
	m["work/specs/"+id+".json"] = specText(id)
	return it(id, typ, parent, "done", `"spec_sha256": "`+hashing.Sum([]byte(specText(id)))+`"`)
}

func arch(items ...string) string {
	return `{"initiative": "I1", "items": [` + strings.Join(items, ",") + `]}`
}

func world(next string, items []string, extra memSource) memSource {
	m := memSource{"backlog.json": backlogWith(next, items...), "canon/config.json": config}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func baseItems(more ...string) []string {
	return append([]string{itemI1, itemF1, itemS1}, more...)
}

// expect checks that src gives exactly one violation per group, each one
// containing every part of its group.
func expect(t *testing.T, src Source, groups ...[]string) {
	t.Helper()
	code, ls := lines(t, src)
	if len(groups) == 0 {
		if code != 0 {
			t.Fatalf("se esperaba código 0 y salió 1: %q", ls)
		}
		return
	}
	if code != 1 {
		t.Fatalf("se esperaba código 1 y salió 0")
	}
	if len(ls) != len(groups) {
		t.Fatalf("se esperaban %d violaciones y hay %d: %q", len(groups), len(ls), ls)
	}
	used := make([]bool, len(ls))
	for _, g := range groups {
		found := false
		for i, l := range ls {
			if used[i] {
				continue
			}
			ok := true
			for _, p := range g {
				if !strings.Contains(l, p) {
					ok = false
				}
			}
			if ok {
				used[i], found = true, true
				break
			}
		}
		if !found {
			t.Errorf("ninguna violación contiene %q; hay %q", g, ls)
		}
	}
	noLibraryText(t, ls)
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVerifyBacklogRules(t *testing.T) {
	t.Run("contador atrasado respecto de un ID del backlog", func(t *testing.T) {
		src := world(`{"I": 100, "F": 100, "S": 5, "B": 100, "N": 100, "C": 100}`, baseItems(it("S38", "slice", "F1", "proposed")), nil)
		expect(t, src, []string{"backlog.json", "next_ids.S", "5", "S38"})
	})

	t.Run("contador atrasado respecto de un ID archivado", func(t *testing.T) {
		m := memSource{}
		s2 := done(m, "S2", "slice", "F1")
		b6 := done(m, "B6", "bug", "S2")
		m["archive/I1.json"] = arch(s2, b6)
		src := world(`{"I": 100, "F": 100, "S": 100, "B": 3, "N": 100, "C": 100}`, baseItems(), m)
		expect(t, src, []string{"backlog.json", "next_ids.B", "3", "B6", "archive/I1.json"})
	})

	t.Run("contador de capacidades", func(t *testing.T) {
		src := world(`{"I": 100, "F": 100, "S": 100, "B": 100, "N": 100, "C": 1}`, baseItems(), nil)
		expect(t, src, []string{"next_ids.C", "1", "C1"})
	})

	t.Run("contador igual al mayor ID", func(t *testing.T) {
		items := baseItems(it("S47", "slice", "F1", "proposed"))
		expect(t, world(`{"I": 100, "F": 100, "S": 47, "B": 100, "N": 100, "C": 100}`, items, nil), []string{"next_ids.S", "47", "S47"})
		expect(t, world(`{"I": 100, "F": 100, "S": 48, "B": 100, "N": 100, "C": 100}`, items, nil))
	})

	t.Run("IDs mal formados", func(t *testing.T) {
		for _, id := range []string{"s3", "S03", "S0", "S-1", "X1", "S", "S99999999999999999999"} {
			src := world(bigNext, baseItems(it(id, "slice", "F1", "proposed")), nil)
			expect(t, src, []string{id})
		}
	})

	t.Run("letra distinta del tipo", func(t *testing.T) {
		src := world(bigNext, baseItems(it("F3", "slice", "F1", "proposed")), nil)
		expect(t, src, []string{"F3", "slice"})
	})

	t.Run("ID repetido", func(t *testing.T) {
		src := world(bigNext, baseItems(it("S3", "slice", "F1", "proposed"), it("S3", "slice", "F1", "proposed")), nil)
		expect(t, src, []string{"S3", "backlog.json"})

		src = world(bigNext, baseItems(it("S3", "slice", "F1", "proposed")), memSource{"archive/I1.json": arch(it("S3", "slice", "F1", "proposed"))})
		expect(t, src, []string{"S3", "backlog.json", "archive/I1.json"})
	})

	t.Run("padres inválidos", func(t *testing.T) {
		cases := []struct {
			name  string
			item  string
			parts []string
		}{
			{"slice con padre iniciativa", it("S2", "slice", "I1", "proposed"), []string{"S2", "I1", "feature"}},
			{"slice con padre inexistente", it("S2", "slice", "F99", "proposed"), []string{"S2", "F99", "feature"}},
			{"feature con padre feature", it("F2", "feature", "F2", "proposed"), []string{"F2", "iniciativa"}},
			{"bug con padre slice proposed", it("B1", "bug", "S1", "proposed"), []string{"B1", "S1", "done"}},
			{"iniciativa con padre", it("I2", "initiative", "F1", "proposed"), []string{"I2", "F1", "null"}},
			{"slice sin padre", it("S2", "slice", "", "proposed"), []string{"S2", "feature"}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				expect(t, world(bigNext, baseItems(c.item), nil), c.parts)
			})
		}
		// A bug under a done slice is valid.
		m2 := memSource{}
		item := done(m2, "S3", "slice", "F1")
		expect(t, world(bigNext, baseItems(item, it("B1", "bug", "S3", "proposed")), m2))
	})

	t.Run("padre archivado", func(t *testing.T) {
		m := memSource{"archive/I1.json": arch(it("F1", "feature", "I1", "done")), "docs/governance/C1-install.md": "# C1\n"}
		items := []string{itemI1, it("S2", "slice", "F1", "proposed"), it("N2", "need", "F1", "proposed")}
		expect(t, world(bigNext, items, m))
	})

	t.Run("archivo archivado ilegible", func(t *testing.T) {
		items := []string{itemI1, it("S2", "slice", "F1", "proposed"), it("S3", "slice", "F1", "proposed"), it("N2", "need", "F1", "proposed")}
		code, ls := lines(t, world(bigNext, items, memSource{"archive/I1.json": "{"}))
		if code != 1 || len(ls) == 0 {
			t.Fatalf("código %d, mensajes %q", code, ls)
		}
		for _, l := range ls {
			mustContain(t, l, "archive/I1.json")
			if strings.Contains(l, "no existe") {
				t.Errorf("cascada de padre inexistente: %q", l)
			}
		}
		if len(ls) != 1 {
			t.Errorf("se esperaba una sola violación: %q", ls)
		}
	})

	t.Run("un slice o bug en curso", func(t *testing.T) {
		items := func() []string {
			return []string{itemI1, itemF1, it("S1", "slice", "F1", "in_progress")}
		}
		two := append(items(), it("S2", "slice", "F1", "in_progress"))
		expect(t, world(bigNext, two, nil), []string{"S1", "S2", "in_progress"})

		m := memSource{}
		s3 := done(m, "S3", "slice", "F1")
		withBug := append(items(), s3, it("B1", "bug", "S3", "in_progress"))
		expect(t, world(bigNext, withBug, m), []string{"S1", "B1", "in_progress"})

		withInit := []string{it("I1", "initiative", "", "in_progress"), itemF1, it("S1", "slice", "F1", "in_progress")}
		expect(t, world(bigNext, withInit, nil))
	})

	t.Run("supersedes inválido", func(t *testing.T) {
		cases := []struct {
			name, target string
			parts        []string
		}{
			{"a sí mismo", "S2", []string{"S2", "supersedes"}},
			{"a un slice proposed", "S1", []string{"S2", "S1", "done"}},
			{"a un ID inexistente", "S999", []string{"S2", "S999"}},
			{"a una feature", "F1", []string{"S2", "F1", "slice"}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				item := it("S2", "slice", "F1", "proposed", `"supersedes": "`+c.target+`"`)
				expect(t, world(bigNext, baseItems(item), nil), c.parts)
			})
		}
		t.Run("en un elemento que no es slice", func(t *testing.T) {
			m := memSource{}
			s3 := done(m, "S3", "slice", "F1")
			item := it("F2", "feature", "I1", "proposed", `"supersedes": "S3"`)
			expect(t, world(bigNext, baseItems(s3, item), m), []string{"F2", "supersedes"})
		})
	})

	t.Run("supersedes hacia un slice done archivado", func(t *testing.T) {
		m := memSource{}
		s7 := done(m, "S7", "slice", "F1")
		m["archive/I1.json"] = arch(s7)
		item := it("S8", "slice", "F1", "proposed", `"supersedes": "S7"`)
		expect(t, world(bigNext, baseItems(item), m))
	})

	t.Run("spec alterada con un espacio", func(t *testing.T) {
		m := memSource{}
		s7 := done(m, "S7", "slice", "F1")
		expect(t, world(bigNext, baseItems(s7), m))
		m["work/specs/S7.json"] += " "
		expect(t, world(bigNext, baseItems(s7), m), []string{"S7", "work/specs/S7.json", "spec_sha256"})
	})

	t.Run("spec con finales CRLF", func(t *testing.T) {
		m := memSource{}
		s7 := done(m, "S7", "slice", "F1")
		m["work/specs/S7.json"] = strings.ReplaceAll(m["work/specs/S7.json"], "\n", "\r\n")
		expect(t, world(bigNext, baseItems(s7), m))
	})

	t.Run("huella inválida o fuera de lugar", func(t *testing.T) {
		m := memSource{}
		m["work/specs/S5.json"] = specText("S5")
		hash := hashing.Sum([]byte(specText("S2")))
		items := baseItems(
			it("S2", "slice", "F1", "done"),
			it("S3", "slice", "F1", "done", `"spec_sha256": "`+hash+`"`),
			it("S4", "slice", "F1", "proposed", `"spec_sha256": "`+hash+`"`),
			it("S5", "slice", "F1", "done", `"spec_sha256": "abc"`),
		)
		expect(t, world(bigNext, items, m),
			[]string{"S2", "spec_sha256"},
			[]string{"S3", "work/specs/S3.json"},
			[]string{"S4", "spec_sha256"},
			[]string{"S5", "spec_sha256", "abc"},
		)
	})

	t.Run("una feature no lleva huella", func(t *testing.T) {
		f := it("F2", "feature", "I1", "proposed", `"spec_sha256": "`+hashing.Sum([]byte("x"))+`"`)
		expect(t, world(bigNext, baseItems(f), nil), []string{"F2", "spec_sha256"})
	})

	t.Run("necesidades por refinar", func(t *testing.T) {
		m := memSource{"work/requests/N5.json": `{"id": "N5", "date": "d", "type": "need", "raw_message": "m"}`}
		items := baseItems(
			it("N2", "need", "", "to_refine"),
			it("N3", "need", "", "to_refine", `"source": "work/requests/N99.json"`),
			it("N4", "need", "", "to_refine", `"source": "../fuera.json"`),
			it("N5", "need", "", "to_refine", `"source": "work/requests/N5.json"`),
			it("N6", "need", "", "to_refine", `"source": "work/requests/../../backlog.json"`),
		)
		expect(t, world(bigNext, items, m),
			[]string{"N2", "source"}, []string{"N3", "N99"}, []string{"N4", "../fuera.json"}, []string{"N6", "source"})
	})
}

func gitRepo(t *testing.T, root string, files map[string]string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no está instalado")
	}
	writeTree(t, root, files)
	git(t, root, "init", "-q")
	git(t, root, "add", ".")
}

func TestVerifyBacklogRulesStagedReadsIndexNotDisk(t *testing.T) {
	m := memSource{}
	s7 := done(m, "S7", "slice", "F1")
	files := map[string]string{"backlog.json": backlogWith(bigNext, baseItems(s7)...), "canon/config.json": config}
	for k, v := range m {
		files[k] = v
	}
	root := t.TempDir()
	gitRepo(t, root, files)

	altered := strings.Replace(m["work/specs/S7.json"], `"n"`, `"m"`, 1)
	writeTree(t, root, map[string]string{"work/specs/S7.json": altered})
	git(t, root, "add", "work/specs/S7.json")
	writeTree(t, root, map[string]string{"work/specs/S7.json": m["work/specs/S7.json"]}) // restored on disk only

	idx, err := Index(root)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, idx, []string{"S7", "work/specs/S7.json", "spec_sha256"})
	expect(t, Disk(root))
}

func TestVerifyBacklogRulesStagedIgnoresInheritedGitDir(t *testing.T) {
	m := memSource{}
	s7 := done(m, "S7", "slice", "F1")
	good := map[string]string{"backlog.json": backlogWith(bigNext, baseItems(s7)...), "canon/config.json": config}
	for k, v := range m {
		good[k] = v
	}
	other := t.TempDir()
	gitRepo(t, other, good)

	local := t.TempDir()
	bad := map[string]string{}
	for k, v := range good {
		bad[k] = v
	}
	bad["backlog.json"] = backlogWith(bigNext, baseItems(it("S2", "slice", "F1", "in_progress"), it("S3", "slice", "F1", "in_progress"))...)
	gitRepo(t, local, bad)

	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	idx, err := Index(local)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, idx, []string{"S2", "S3", "in_progress"})
}

func TestVerifyBacklogRulesStagedWithoutGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no está instalado")
	}
	files := map[string]string{"backlog.json": backlog(), "canon/config.json": config}

	plain := t.TempDir()
	writeTree(t, plain, files)
	_, err := Index(plain)
	if err == nil {
		t.Fatal("sin .git debe fallar")
	}
	mustContain(t, err.Error(), "no es un repositorio git")

	// A folder without its own .git inside another repository is not a repo of its own.
	outer := t.TempDir()
	gitRepo(t, outer, map[string]string{"x.txt": "x"})
	inner := filepath.Join(outer, "instalada")
	writeTree(t, inner, files)
	_, err = Index(inner)
	if err == nil {
		t.Fatal("una carpeta sin .git propio dentro de otro repo debe fallar")
	}
	mustContain(t, err.Error(), "no es un repositorio git")
}
