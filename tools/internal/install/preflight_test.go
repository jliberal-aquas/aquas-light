package install

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func okDeps() Deps {
	return Deps{
		GoVersion: func() (string, error) { return "go1.24.7", nil },
		LookGit:   func() error { return nil },
	}
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range list {
		names = append(names, e.Name())
	}
	return names
}

func TestPreflightRejectsNonEmptyFolder(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"README.md": func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		".gitkeep": func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, ".gitkeep"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		},
		".git": func(t *testing.T, dir string) {
			if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"a": func(t *testing.T, dir string) {
			if err := os.Mkdir(filepath.Join(dir, "a"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			setup(t, dir)
			before := entries(t, dir)
			err := Preflight(dir, okDeps())
			if err == nil {
				t.Fatal("se esperaba un error por carpeta no vacía")
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("el mensaje debe nombrar %q: %q", name, err)
			}
			if got := entries(t, dir); strings.Join(got, ",") != strings.Join(before, ",") {
				t.Errorf("la carpeta cambió: %v -> %v", before, got)
			}
		})
	}
}

func TestPreflightCreatesMissingAndAcceptsEmpty(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "nuevo", "dest")
	if err := Preflight(missing, okDeps()); err != nil {
		t.Fatal(err)
	}
	if n := len(entries(t, missing)); n != 0 {
		t.Errorf("entradas = %d", n)
	}
	if err := Preflight(missing, okDeps()); err != nil {
		t.Errorf("carpeta vacía existente debe aceptarse: %v", err)
	}
}

func TestPreflightMissingGit(t *testing.T) {
	root := t.TempDir()
	deps := okDeps()
	deps.LookGit = func() error { return errors.New("not found") }

	missing := filepath.Join(root, "x")
	err := Preflight(missing, deps)
	if err == nil || !strings.Contains(err.Error(), "git") {
		t.Fatalf("error = %v", err)
	}
	if _, e := os.Stat(missing); !os.IsNotExist(e) {
		t.Error("la ruta no debía crearse")
	}

	empty := filepath.Join(root, "vacia")
	if e := os.Mkdir(empty, 0o755); e != nil {
		t.Fatal(e)
	}
	if err := Preflight(empty, deps); err == nil || !strings.Contains(err.Error(), "git") {
		t.Fatalf("error = %v", err)
	}
	if n := len(entries(t, empty)); n != 0 {
		t.Errorf("entradas = %d", n)
	}
}

func TestPreflightGoVersion(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "x")
	for _, v := range []string{"go1.10", "go1.9.2", "devel go1.30-abc", "basura"} {
		deps := okDeps()
		deps.GoVersion = func() (string, error) { return v, nil }
		err := Preflight(dest, deps)
		if err == nil || !strings.Contains(err.Error(), v) || !strings.Contains(err.Error(), "1.24") {
			t.Errorf("%s: error = %v", v, err)
		}
		if _, e := os.Stat(dest); !os.IsNotExist(e) {
			t.Errorf("%s: la ruta no debía crearse", v)
		}
	}
	deps := okDeps()
	deps.GoVersion = func() (string, error) { return "", errors.New("no go") }
	if err := Preflight(dest, deps); err == nil || !strings.Contains(err.Error(), "Go") {
		t.Errorf("go ausente: %v", err)
	}
	for _, v := range []string{"go1.24", "go1.24.7", "go1.25rc1", "go1.100.0", "go1.30"} {
		deps := okDeps()
		deps.GoVersion = func() (string, error) { return v, nil }
		if err := Preflight(filepath.Join(root, "ok"+v), deps); err != nil {
			t.Errorf("%s debe aceptarse: %v", v, err)
		}
	}
}
