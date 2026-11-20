package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const page1 = "docs/governance/C1-install-and-update.md"

// capDecl builds one entry of `capabilities`.
func capDecl(id, page string) string {
	return fmt.Sprintf(`{"id": %q, "name": "n", "page": %q}`, id, page)
}

// feat builds a feature item. capField is the raw JSON of the capability
// value; an empty one leaves the field out.
func feat(id, state, capField string) string {
	s := fmt.Sprintf(`{"id": %q, "type": "feature", "parent": "I1", "state": %q, "priority": 1, "spec": null`, id, state)
	if capField != "" {
		s += `, "capability": ` + capField
	}
	return s + "}"
}

func capBacklog(caps []string, items ...string) string {
	return "{\n  \"schema_version\": 1,\n  \"next_ids\": " + bigNext + ",\n  \"capabilities\": [" + strings.Join(caps, ",") +
		"],\n  \"items\": [\n    " + strings.Join(items, ",\n    ") + "\n  ]\n}\n"
}

// capWorld is a repo whose F5 is archived under capability C1.
func capWorld(caps []string, files map[string]string) memSource {
	m := memSource{
		"backlog.json":      capBacklog(caps, itemI1),
		"canon/config.json": config,
		"archive/I1.json":   arch(feat("F5", "done", `"C1"`)),
	}
	for k, v := range files {
		m[k] = v
	}
	return m
}

func TestVerifyCapabilityPages(t *testing.T) {
	ok := map[string]string{page1: "# C1\n"}

	t.Run("la página existe", func(t *testing.T) {
		expect(t, capWorld([]string{capDecl("C1", page1)}, ok))
	})

	t.Run("página ausente", func(t *testing.T) {
		expect(t, capWorld([]string{capDecl("C1", page1)}, nil), []string{"backlog.json", "capabilities[0].page", "C1", page1})
	})

	t.Run("directorio y archivo vacío", func(t *testing.T) {
		m := capWorld([]string{capDecl("C1", page1), capDecl("C2", "docs/governance/C2-verify.md")}, map[string]string{
			page1 + "/dentro.md":           "x",
			"docs/governance/C2-verify.md": "",
			"archive/I1.json":              arch(feat("F5", "done", `"C1"`), feat("F6", "done", `"C2"`)),
		})
		expect(t, m, []string{"C1", page1, "directorio"}, []string{"C2", "C2-verify.md", "vacío"})
	})

	t.Run("rutas que escapan o no son relativas", func(t *testing.T) {
		for _, p := range []string{"../fuera.md", "/etc/hosts", `C:\docs\C1.md`, "docs/governance/../../backlog.json"} {
			m := capWorld([]string{capDecl("C1", p)}, map[string]string{"fuera.md": "x", "backlog.json2": "x"})
			expect(t, m, []string{"C1", "capabilities[0].page"})
		}
	})

	t.Run("barras invertidas aunque el archivo exista", func(t *testing.T) {
		p := `docs\governance\C1-install-and-update.md`
		expect(t, capWorld([]string{capDecl("C1", p)}, map[string]string{p: "x", page1: "x"}), []string{"C1", "barras"})
	})

	t.Run("fuera de docs/governance y docs/product", func(t *testing.T) {
		m := capWorld([]string{capDecl("C1", "README.md")}, map[string]string{"README.md": "x"})
		expect(t, m, []string{"C1", "README.md"})
		m = capWorld([]string{capDecl("C1", "docs/C1-x.md")}, map[string]string{"docs/C1-x.md": "x"})
		expect(t, m, []string{"C1", "docs/C1-x.md"})
	})

	t.Run("docs/product es válido", func(t *testing.T) {
		p := "docs/product/C1-install.md"
		expect(t, capWorld([]string{capDecl("C1", p)}, map[string]string{p: "x"}))
	})

	t.Run("página de otra capacidad", func(t *testing.T) {
		m := capWorld([]string{capDecl("C1", "docs/governance/C2-verify.md")}, map[string]string{"docs/governance/C2-verify.md": "x"})
		expect(t, m, []string{"C1", "C2-verify.md"})
	})

	t.Run("mayúsculas distintas en disco", func(t *testing.T) {
		for _, tc := range []struct{ name, onDisk string }{
			{"archivo", "docs/governance/c1-install-and-update.md"},
			{"carpeta", "docs/Governance/C1-install-and-update.md"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				root := t.TempDir()
				writeTree(t, root, map[string]string{
					"backlog.json":      capBacklog([]string{capDecl("C1", page1)}, itemI1),
					"canon/config.json": config,
					"archive/I1.json":   arch(feat("F5", "done", `"C1"`)),
					tc.onDisk:           "x",
				})
				expect(t, Disk(root), []string{"C1", page1})
			})
		}
	})

	t.Run("enlace simbólico a un archivo de fuera", func(t *testing.T) {
		root := t.TempDir()
		outside := filepath.Join(t.TempDir(), "fuera.md")
		if err := os.WriteFile(outside, []byte("secreto"), 0o644); err != nil {
			t.Fatal(err)
		}
		writeTree(t, root, map[string]string{
			"backlog.json":      capBacklog([]string{capDecl("C1", page1)}, itemI1),
			"canon/config.json": config,
			"archive/I1.json":   arch(feat("F5", "done", `"C1"`)),
		})
		if err := os.MkdirAll(filepath.Join(root, "docs", "governance"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, filepath.FromSlash(page1))); err != nil {
			t.Skip("no se pueden crear enlaces simbólicos aquí: ", err)
		}
		expect(t, Disk(root), []string{"C1", "enlace"})
	})

	t.Run("features con capacidad inválida", func(t *testing.T) {
		m := memSource{
			"backlog.json": capBacklog([]string{capDecl("C1", page1)}, itemI1,
				feat("F2", "ready", `"C99"`), feat("F3", "ready", ""), feat("F4", "ready", `"c1"`)),
			"canon/config.json": config,
		}
		expect(t, m, []string{"backlog.json", "F2", "C99"}, []string{"backlog.json", "F3", "capability"}, []string{"backlog.json", "F4", "c1"})
	})

	t.Run("feature archivada con capacidad inexistente", func(t *testing.T) {
		m := capWorld([]string{capDecl("C1", page1)}, ok)
		m["archive/I1.json"] = arch(feat("F5", "done", `"C99"`))
		expect(t, m, []string{"archive/I1.json", "F5", "C99"})
	})

	t.Run("ID y página repetidos", func(t *testing.T) {
		m := memSource{
			"backlog.json":      capBacklog([]string{capDecl("C1", page1), capDecl("C1", "docs/governance/C1-otra.md")}, itemI1),
			"canon/config.json": config,
		}
		expect(t, m, []string{"C1", "repetido", "capabilities[1]"})
		m["backlog.json"] = capBacklog([]string{capDecl("C1", page1), capDecl("C2", page1)}, itemI1)
		expect(t, m, []string{"C2", page1, "repetida", "capabilities[1]"})
	})

	t.Run("plantilla vacía", func(t *testing.T) {
		m := memSource{"backlog.json": capBacklog(nil), "canon/config.json": config}
		expect(t, m)
	})

	t.Run("capacidad sin features archivadas no exige página", func(t *testing.T) {
		m := memSource{
			"backlog.json":      capBacklog([]string{capDecl("C1", page1), capDecl("C3", "docs/governance/C3-x.md")}, itemI1, feat("F2", "ready", `"C3"`)),
			"canon/config.json": config,
		}
		expect(t, m)
	})

	t.Run("staged: la página está en disco y no en el índice", func(t *testing.T) {
		root := t.TempDir()
		files := map[string]string{
			"backlog.json":      capBacklog([]string{capDecl("C1", page1)}, itemI1),
			"canon/config.json": config,
			"archive/I1.json":   arch(feat("F5", "done", `"C1"`)),
		}
		gitRepo(t, root, files)
		writeTree(t, root, map[string]string{page1: "x"})
		idx, err := Index(root)
		if err != nil {
			t.Fatal(err)
		}
		expect(t, idx, []string{"C1", page1})
		expect(t, Disk(root))
		git(t, root, "add", page1)
		idx, err = Index(root)
		if err != nil {
			t.Fatal(err)
		}
		expect(t, idx)
	})
}
