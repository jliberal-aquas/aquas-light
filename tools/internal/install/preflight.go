// Package install holds the logic behind "aquas install".
package install

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Minimum supported Go version (keep in sync with canon/toolchain.json).
const (
	minGoMajor = 1
	minGoMinor = 24
	minGoText  = "1.24"
)

// Deps are the injectable dependencies of Preflight.
type Deps struct {
	// GoVersion returns the Go version found, such as "go1.24.7".
	GoVersion func() (string, error)
	// LookGit returns nil when git can be found.
	LookGit func() error
}

// DefaultDeps returns dependencies that inspect the real environment.
func DefaultDeps() Deps {
	return Deps{
		GoVersion: func() (string, error) {
			out, err := exec.Command("go", "env", "GOVERSION").Output()
			return strings.TrimSpace(string(out)), err
		},
		LookGit: func() error {
			_, err := exec.LookPath("git")
			return err
		},
	}
}

var goVersionRe = regexp.MustCompile(`^go(\d+)\.(\d+)`)

func checkGo(deps Deps) error {
	v, err := deps.GoVersion()
	if err != nil {
		return fmt.Errorf("no se encontró Go (se requiere %s o superior): %v", minGoText, err)
	}
	m := goVersionRe.FindStringSubmatch(v)
	if m == nil {
		return fmt.Errorf("no se pudo interpretar la versión de Go encontrada %q (mínima soportada: %s)", v, minGoText)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	if major < minGoMajor || (major == minGoMajor && minor < minGoMinor) {
		return fmt.Errorf("la versión de Go encontrada es %s y la mínima soportada es %s", v, minGoText)
	}
	return nil
}

// resolvePath returns p as an absolute, clean path with symlinks resolved in
// its longest existing prefix (the rest may not exist yet).
func resolvePath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	var rest []string
	cur := abs
	for {
		if r, e := filepath.EvalSymlinks(cur); e == nil {
			for i := len(rest) - 1; i >= 0; i-- {
				r = filepath.Join(r, rest[i])
			}
			return r, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, nil
		}
		rest = append(rest, filepath.Base(cur))
		cur = parent
	}
}

// checkOutsidePackage rejects a dest equal to root or inside it.
func checkOutsidePackage(root, dest string) error {
	r, err := resolvePath(root)
	if err != nil {
		return fmt.Errorf("ruta inválida %s: %v", root, err)
	}
	d, err := resolvePath(dest)
	if err != nil {
		return fmt.Errorf("ruta inválida %s: %v", dest, err)
	}
	cr, cd := r, d
	if runtime.GOOS == "windows" {
		cr, cd = strings.ToLower(r), strings.ToLower(d)
	}
	rel, err := filepath.Rel(cr, cd)
	if err != nil {
		return nil
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return fmt.Errorf("el destino no puede ser la raíz del paquete ni estar dentro de ella (%s): elige una carpeta fuera de ese paquete", root)
	}
	return nil
}

// Preflight checks the preconditions of an install into dest. It creates dest
// when missing and requires it to be empty when it already exists. It never
// modifies a folder that already existed, and removes what it created on failure.
func Preflight(dest string, deps Deps) error {
	return PreflightIn("", dest, deps)
}

// PreflightIn is Preflight plus the package root: when root is not empty, a
// dest equal to root or inside it is rejected before anything is created.
func PreflightIn(root, dest string, deps Deps) (err error) {
	if root != "" {
		if e := checkOutsidePackage(root, dest); e != nil {
			return e
		}
	}
	var problems []error
	if e := deps.LookGit(); e != nil {
		problems = append(problems, errors.New("no se encontró `git` en el PATH; instálalo antes de continuar"))
	}
	if e := checkGo(deps); e != nil {
		problems = append(problems, e)
	}
	if len(problems) > 0 {
		return errors.Join(problems...)
	}

	info, statErr := os.Stat(dest)
	switch {
	case statErr == nil:
		if !info.IsDir() {
			return fmt.Errorf("la ruta %s existe y no es una carpeta", dest)
		}
		list, e := os.ReadDir(dest)
		if e != nil {
			return fmt.Errorf("no se pudo leer la carpeta %s: %v", dest, e)
		}
		if len(list) > 0 {
			return fmt.Errorf("la carpeta %s no está vacía (contiene %q); elige una carpeta vacía o una ruta nueva", dest, list[0].Name())
		}
		return nil
	case !errors.Is(statErr, os.ErrNotExist):
		return fmt.Errorf("no se pudo inspeccionar %s: %v", dest, statErr)
	}

	// Remember the topmost missing ancestor so a failure removes only what we made.
	abs, e := filepath.Abs(dest)
	if e != nil {
		return fmt.Errorf("ruta inválida %s: %v", dest, e)
	}
	top := abs
	for p := filepath.Dir(abs); p != top; p, top = filepath.Dir(p), p {
		if _, e := os.Stat(p); e == nil {
			break
		}
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(top)
		}
	}()
	if e := os.MkdirAll(abs, 0o755); e != nil {
		return fmt.Errorf("no se pudo crear la carpeta %s: %v", dest, e)
	}
	return nil
}
