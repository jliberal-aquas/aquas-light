package install

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestPreflightRejectsDestInsidePackage(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"canon", "tools"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "tools", "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(root, "tools"))

	check := func(name, dest string, mustNotExist string) {
		t.Helper()
		err := PreflightIn(root, dest, okDeps())
		if err == nil {
			t.Errorf("%s: se esperaba un error", name)
		} else {
			if len(err.Error()) >= 300 {
				t.Errorf("%s: mensaje de %d caracteres", name, len(err.Error()))
			}
			if !strings.Contains(err.Error(), root) {
				t.Errorf("%s: el mensaje no nombra la raíz %s: %q", name, root, err)
			}
		}
		if mustNotExist != "" {
			if _, e := os.Stat(mustNotExist); e == nil {
				t.Errorf("%s: %s no debía crearse", name, mustNotExist)
			}
		}
	}
	check("relative inside tools", "mi-api", filepath.Join(root, "tools", "mi-api"))
	check("dest equals root", root, "")
	check("dest nested deep", filepath.Join(root, "a", "b"), filepath.Join(root, "a"))
	check("dest with dotdot", filepath.Join("..", "..", filepath.Base(root), "x"), filepath.Join(root, "x"))
	if runtime.GOOS == "windows" {
		check("case-insensitive", strings.ToUpper(filepath.Join(root, "zz")), "")
	}

	// A sibling outside the package is still accepted.
	outside := filepath.Join(t.TempDir(), "ok")
	if err := PreflightIn(root, outside, okDeps()); err != nil {
		t.Errorf("destino fuera del paquete rechazado: %v", err)
	}
	// A sibling with the same name prefix is outside too.
	prefixed := root + "-copia"
	t.Cleanup(func() { _ = os.RemoveAll(prefixed) })
	if err := PreflightIn(root, prefixed, okDeps()); err != nil {
		t.Errorf("destino hermano con prefijo igual rechazado: %v", err)
	}
}
