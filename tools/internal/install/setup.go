package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Setup prepares a clone of an instance: it builds bin/aquas, downloads
// golangci-lint (SHA-256 verified), writes work/host.json and sets the local
// core.hooksPath to .githooks. It never commits and is idempotent. On failure
// it keeps the repository untouched and leaves no half-written binary.
func Setup(repoRoot string, opts Options) error {
	opts = opts.withDefaults()
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("ruta inválida %s: %v", repoRoot, err)
	}
	if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s no es un repositorio git (falta .git): clona la instancia con `git clone` y ejecuta `setup` dentro del clon", abs)
		}
		return fmt.Errorf("no se pudo leer .git en %s: %v", abs, err)
	}
	if err := buildAquas(abs); err != nil {
		cleanBin(abs, "aquas")
		return err
	}
	if err := downloadLint(abs, opts); err != nil {
		return err
	}
	if err := writeHost(abs); err != nil {
		return err
	}
	if out, err := git(abs, "config", "--local", "core.hooksPath", ".githooks"); err != nil {
		return fmt.Errorf("falló `git config --local core.hooksPath .githooks`: %v\n%s", err, out)
	}
	return nil
}

// cleanBin removes a binary that may have been left half-written.
func cleanBin(dest, name string) {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	_ = os.Remove(filepath.Join(dest, "bin", name))
}
