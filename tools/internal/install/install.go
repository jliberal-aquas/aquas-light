package install

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jliberal-aquas/aquas-light/tools/internal/host"
	"github.com/jliberal-aquas/aquas-light/tools/internal/verify"
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
	// Verify checks the governed files of the repo at root; nil uses the real
	// verifier. Run calls it once, in disk mode, before git add.
	Verify func(root string) ([]verify.Violation, error)
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
// build bin/aquas, download golangci-lint, write work/host.json, verify the
// result and make the root commit. On any failure it removes everything it created.
func Run(root, dest string, opts Options) (err error) {
	opts = opts.withDefaults()
	abs, err := filepath.Abs(dest)
	if err != nil {
		return fmt.Errorf("ruta inválida %s: %v", dest, err)
	}
	_, statErr := os.Stat(abs)
	existed := statErr == nil
	top := topMissing(abs)

	if err := PreflightIn(root, dest, opts.Deps); err != nil {
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
	if err := runVerify(abs, opts); err != nil {
		return err
	}
	return commitInstall(abs, opts.Version)
}

// runVerify checks the installed files in disk mode. Violations and run errors
// both fail the install, with the verifier's own message.
func runVerify(abs string, opts Options) error {
	check := opts.Verify
	if check == nil {
		check = func(root string) ([]verify.Violation, error) { return verify.Run(verify.Disk(root)) }
	}
	vs, err := check(abs)
	if err != nil {
		return fmt.Errorf("no se pudo verificar lo instalado: %v", err)
	}
	if len(vs) == 0 {
		return nil
	}
	lines := make([]string, len(vs))
	for i, v := range vs {
		lines[i] = v.String()
	}
	return fmt.Errorf("lo instalado no pasa la verificación:\n%s", strings.Join(lines, "\n"))
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
