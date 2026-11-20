package install

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jliberal-aquas/aquas-light/tools/internal/verify"
)

// copyPkg copies the package folders Run reads into a new root, so a test can
// break one of its files.
func copyPkg(t *testing.T) string {
	t.Helper()
	src, dst := pkgRoot(t), t.TempDir()
	for _, top := range []string{"canon", "schemas", "tools", "templates"} {
		base := filepath.Join(src, top)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(src, p)
			if err != nil {
				return err
			}
			if d.IsDir() {
				if filepath.ToSlash(rel) == "tools/bin" {
					return filepath.SkipDir
				}
				return os.MkdirAll(filepath.Join(dst, rel), 0o755)
			}
			if !d.Type().IsRegular() {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

func writeIn(t *testing.T, root, rel, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const brokenTemplate = `{
  "schema_version": 1,
  "next_ids": {"I": 1, "F": 1, "S": 1, "B": 1, "N": 1, "C": 1},
  "capabilities": [],
  "items": [],
  "color": "rojo"
}
`

func entryNames(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range list {
		out = append(out, e.Name())
	}
	return out
}

func runAquas(t *testing.T, dir string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(filepath.Join(dir, "bin", "aquas"+exeSuffix()), args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0, string(out)
	case errors.As(err, &ee):
		return ee.ExitCode(), string(out)
	}
	t.Fatalf("no se pudo ejecutar bin/aquas: %v", err)
	return -1, ""
}

func TestInstallRunsVerify(t *testing.T) {
	gitEnv(t, identity)
	url, sums := fakeLint(t)

	t.Run("plantilla rota en una ruta que no existe", func(t *testing.T) {
		pkg := copyPkg(t)
		writeIn(t, pkg, "templates/backlog.json", brokenTemplate)
		dest := filepath.Join(t.TempDir(), "nuevo")
		err := Run(pkg, dest, okOptions(url, sums))
		if err == nil {
			t.Fatal("se esperaba un error")
		}
		if !strings.Contains(err.Error(), "backlog.json") || !strings.Contains(err.Error(), "color") {
			t.Errorf("el mensaje debe nombrar backlog.json y color: %q", err)
		}
		if _, e := os.Stat(dest); e == nil {
			t.Error("la carpeta destino no debía existir")
		}
	})

	t.Run("plantilla rota en una carpeta vacía existente", func(t *testing.T) {
		pkg := copyPkg(t)
		writeIn(t, pkg, "templates/backlog.json", brokenTemplate)
		dest := t.TempDir()
		if err := Run(pkg, dest, okOptions(url, sums)); err == nil {
			t.Fatal("se esperaba un error")
		}
		if got := entryNames(t, dest); len(got) != 0 {
			t.Errorf("la carpeta debía quedar vacía y tiene %v", got)
		}
	})

	t.Run("el verificador corre antes del commit y después de bin y host", func(t *testing.T) {
		var headErr error
		var binOK, hostOK bool
		opts := okOptions(url, sums)
		opts.Verify = func(root string) ([]verify.Violation, error) {
			_, headErr = git(root, "rev-parse", "--verify", "HEAD")
			_, e1 := os.Stat(filepath.Join(root, "bin", "aquas"+exeSuffix()))
			_, e2 := os.Stat(filepath.Join(root, "work", "host.json"))
			binOK, hostOK = e1 == nil, e2 == nil
			return nil, nil
		}
		if err := Run(pkgRoot(t), filepath.Join(t.TempDir(), "x"), opts); err != nil {
			t.Fatal(err)
		}
		if headErr == nil {
			t.Error("HEAD no debía existir todavía")
		}
		if !binOK || !hostOK {
			t.Errorf("bin/aquas existe=%v, work/host.json existe=%v; ambos debían existir", binOK, hostOK)
		}
	})

	t.Run("error de ejecución del verificador", func(t *testing.T) {
		opts := okOptions(url, sums)
		opts.Verify = func(string) ([]verify.Violation, error) { return nil, errors.New("disco ilegible") }
		missing := filepath.Join(t.TempDir(), "nuevo")
		err := Run(pkgRoot(t), missing, opts)
		if err == nil || !strings.Contains(err.Error(), "disco ilegible") {
			t.Fatalf("el error debe contener el texto del verificador: %v", err)
		}
		if _, e := os.Stat(missing); e == nil {
			t.Error("la carpeta destino no debía existir")
		}
		existing := t.TempDir()
		if err := Run(pkgRoot(t), existing, opts); err == nil || !strings.Contains(err.Error(), "disco ilegible") {
			t.Fatalf("el error debe contener el texto del verificador: %v", err)
		}
		if got := entryNames(t, existing); len(got) != 0 {
			t.Errorf("la carpeta debía quedar vacía y tiene %v", got)
		}
	})

	t.Run("una sola llamada con la carpeta destino como raíz", func(t *testing.T) {
		var calls []string
		opts := okOptions(url, sums)
		opts.Verify = func(root string) ([]verify.Violation, error) {
			calls = append(calls, root)
			return nil, nil
		}
		dest := filepath.Join(t.TempDir(), "x")
		if err := Run(pkgRoot(t), dest, opts); err != nil {
			t.Fatal(err)
		}
		abs, _ := filepath.Abs(dest)
		if len(calls) != 1 || calls[0] != abs {
			t.Errorf("llamadas = %q, se esperaba exactamente [%q]", calls, abs)
		}
	})

	t.Run("el backlog.json de la raíz del paquete no cuenta", func(t *testing.T) {
		pkg := copyPkg(t)
		writeIn(t, pkg, "backlog.json", "esto no es JSON")
		if err := Run(pkg, filepath.Join(t.TempDir(), "x"), okOptions(url, sums)); err != nil {
			t.Fatalf("la instalación debía salir bien: %v", err)
		}
	})

	t.Run("instalación correcta y verify dentro del repo", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "x")
		if err := Run(pkgRoot(t), dest, okOptions(url, sums)); err != nil {
			t.Fatal(err)
		}
		if code, out := runAquas(t, dest, "verify"); code != 0 {
			t.Errorf("verify salió con %d: %s", code, out)
		}
		if got := gitOut(t, dest, "rev-list", "--count", "HEAD"); got != "1" {
			t.Errorf("commits = %s, se esperaba 1", got)
		}
		if st := gitOut(t, dest, "status", "--porcelain"); st != "" {
			t.Errorf("el árbol debía quedar limpio: %q", st)
		}
	})

	t.Run("ruta con espacio y ñ", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "mi año", "api")
		if err := Run(pkgRoot(t), dest, okOptions(url, sums)); err != nil {
			t.Fatal(err)
		}
		if code, out := runAquas(t, dest, "verify"); code != 0 {
			t.Errorf("verify salió con %d: %s", code, out)
		}
	})

	t.Run("setup no llama al verificador", func(t *testing.T) {
		clone := cloneOfInstance(t)
		calls := 0
		opts := okOptions(url, sums)
		opts.Verify = func(string) ([]verify.Violation, error) { calls++; return nil, nil }
		if err := Setup(clone, opts); err != nil {
			t.Fatal(err)
		}
		if calls != 0 {
			t.Errorf("setup llamó al verificador %d veces", calls)
		}
		if code, out := runAquas(t, clone, "verify"); code != 0 {
			t.Errorf("verify salió con %d: %s", code, out)
		}
		if st := gitOut(t, clone, "status", "--porcelain"); st != "" {
			t.Errorf("el árbol debía seguir limpio: %q", st)
		}
	})
}
