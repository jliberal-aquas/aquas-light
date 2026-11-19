package install

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/jliberal-aquas/aquas-light/tools/internal/host"
)

// Options configures Run. The zero value installs with real dependencies.
type Options struct {
	// Version is the aquas version used in the install commit subject.
	Version string
	// Deps are the Preflight dependencies; nil funcs use DefaultDeps.
	Deps Deps
	// LintBaseURL overrides the golangci-lint download base URL (tests).
	LintBaseURL string
	// LintChecksums overrides the pinned checksums, keyed like pinnedLintSHA256.
	LintChecksums map[string]string
	// GOOS and GOARCH override the platform used to pick the lint asset.
	GOOS, GOARCH string
	// AfterGitInit, when set, runs right after git init (fault injection).
	AfterGitInit func(dest string) error
}

func (o Options) withDefaults() Options {
	d := DefaultDeps()
	if o.Deps.GoVersion == nil {
		o.Deps.GoVersion = d.GoVersion
	}
	if o.Deps.LookGit == nil {
		o.Deps.LookGit = d.LookGit
	}
	if o.LintBaseURL == "" {
		o.LintBaseURL = defaultLintBaseURL
	}
	if o.LintChecksums == nil {
		o.LintChecksums = pinnedLintSHA256
	}
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.GOARCH == "" {
		o.GOARCH = runtime.GOARCH
	}
	return o
}

// Run installs the package found at root into dest: preflight, git init, copy,
// build bin/aquas, download golangci-lint, write work/host.json and make the
// root commit. On any failure it removes everything it created.
func Run(root, dest string, opts Options) (err error) {
	opts = opts.withDefaults()
	abs, err := filepath.Abs(dest)
	if err != nil {
		return fmt.Errorf("ruta inválida %s: %v", dest, err)
	}
	_, statErr := os.Stat(abs)
	existed := statErr == nil
	top := topMissing(abs)

	if err := Preflight(dest, opts.Deps); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			rollback(abs, top, existed)
		}
	}()

	if err := gitInit(abs); err != nil {
		return err
	}
	if opts.AfterGitInit != nil {
		if err := opts.AfterGitInit(abs); err != nil {
			return err
		}
	}
	if err := Copy(root, abs, opts.Version); err != nil {
		return err
	}
	if err := buildAquas(abs); err != nil {
		return err
	}
	if err := downloadLint(abs, opts); err != nil {
		return err
	}
	if err := writeHost(abs); err != nil {
		return err
	}
	return commitInstall(abs, opts.Version)
}

func buildAquas(dest string) error {
	name := "aquas"
	if runtime.GOOS == "windows" {
		name = "aquas.exe"
	}
	out := filepath.Join(dest, "bin", name)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return fmt.Errorf("no se pudo crear bin/: %v", err)
	}
	cmd := exec.Command("go", "build", "-o", out, "./cmd/aquas")
	cmd.Dir = filepath.Join(dest, "tools")
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("falló `go build` de bin/%s: %v\n%s", name, err, b)
	}
	return nil
}

func writeHost(dest string) error {
	return host.Write(dest, host.Options{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH})
}

var errNoIdentity = errors.New("git no tiene user.name ni user.email configurados; ejecuta `git config --global user.name \"Tu Nombre\"` y `git config --global user.email tu@correo`")
