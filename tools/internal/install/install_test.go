package install

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func pkgRoot(t *testing.T) string {
	t.Helper()
	root, err := FindPackageRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// gitEnv isolates git from the machine config and gives it a global config file
// with the given content.
func gitEnv(t *testing.T, globalConfig string) {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(cfg, []byte(globalConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
}

const identity = "[user]\n\tname = Test\n\temail = test@example.com\n"

func okOptions(url string, sums map[string]string) Options {
	return Options{Version: "0.1.0", Deps: okDeps(), LintBaseURL: url, LintChecksums: sums}
}

// fakeLint serves a fake golangci-lint archive for the host platform and
// returns the server URL and a checksums map matching it.
func fakeLint(t *testing.T) (string, map[string]string) {
	t.Helper()
	name := "golangci-lint"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	suffix := lintSuffix(runtime.GOOS, runtime.GOARCH)
	var buf bytes.Buffer
	content := []byte("binario falso")
	if runtime.GOOS == "windows" {
		zw := zip.NewWriter(&buf)
		w, err := zw.Create("golangci-lint-2.5.0-" + suffix[:len(suffix)-4] + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(content)
		_ = zw.Close()
	} else {
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		_ = tw.WriteHeader(&tar.Header{Name: "golangci-lint-2.5.0/" + name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg})
		_, _ = tw.Write(content)
		_ = tw.Close()
		_ = gz.Close()
	}
	sum := sha256.Sum256(buf.Bytes())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)
	return srv.URL, map[string]string{suffix: hex.EncodeToString(sum[:])}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := git(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return out
}

func TestInstallRollsBackOnFailure(t *testing.T) {
	gitEnv(t, identity)
	dest := filepath.Join(t.TempDir(), "nuevo")
	opts := okOptions("http://127.0.0.1:1", nil)
	opts.AfterGitInit = func(string) error { return errors.New("fallo inyectado") }
	err := Run(pkgRoot(t), dest, opts)
	if err == nil {
		t.Fatal("se esperaba un error por el fallo inyectado")
	}
	if _, e := os.Stat(dest); e == nil {
		t.Error("la carpeta creada por la instalación debía borrarse")
	}
}

func TestInstallSuccess(t *testing.T) {
	gitEnv(t, identity)
	url, sums := fakeLint(t)
	dest := filepath.Join(t.TempDir(), "ok")
	if err := Run(pkgRoot(t), dest, okOptions(url, sums)); err != nil {
		t.Fatal(err)
	}
	if got := gitOut(t, dest, "rev-list", "--count", "HEAD"); got != "1" {
		t.Errorf("commits = %s, se esperaba 1", got)
	}
	if got := gitOut(t, dest, "log", "-1", "--format=%s"); got != "Install Aquas Light v0.1.0" {
		t.Errorf("asunto = %q", got)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	for _, f := range []string{"bin/aquas" + exe, "bin/golangci-lint" + exe, "work/host.json"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(f))); err != nil {
			t.Errorf("falta %s: %v", f, err)
		}
	}
	if st := gitOut(t, dest, "status", "--porcelain"); st != "" {
		t.Errorf("el árbol debía quedar limpio: %q", st)
	}
}

func TestInstallLintServerError(t *testing.T) {
	gitEnv(t, identity)
	_, sums := fakeLint(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	for _, existed := range []bool{false, true} {
		dest := filepath.Join(t.TempDir(), "d")
		if existed {
			if err := os.Mkdir(dest, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		err := Run(pkgRoot(t), dest, okOptions(srv.URL, sums))
		if err == nil || !strings.Contains(err.Error(), "500") {
			t.Fatalf("existed=%v: error = %v, se esperaba mención de 500", existed, err)
		}
		if existed {
			if n := entries(t, dest); len(n) != 0 {
				t.Errorf("la carpeta debía quedar con 0 entradas: %v", n)
			}
		} else if _, e := os.Stat(dest); e == nil {
			t.Error("la carpeta creada debía borrarse")
		}
	}
}

func TestInstallLintChecksumMismatch(t *testing.T) {
	gitEnv(t, identity)
	url, sums := fakeLint(t)
	want := strings.Repeat("ab", 32)
	for k := range sums {
		sums[k] = want
	}
	dest := filepath.Join(t.TempDir(), "d")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	err := Run(pkgRoot(t), dest, okOptions(url, sums))
	if err == nil {
		t.Fatal("se esperaba error por huella distinta")
	}
	if !strings.Contains(err.Error(), want) || !regexp.MustCompile(`recibida [0-9a-f]{64}`).MatchString(err.Error()) {
		t.Errorf("el mensaje debe nombrar esperada y recibida: %v", err)
	}
	if n := entries(t, dest); len(n) != 0 {
		t.Errorf("la carpeta debía quedar con 0 entradas: %v", n)
	}
}

func TestInstallRollbackRemovesReadOnlyGit(t *testing.T) {
	gitEnv(t, identity)
	for _, existed := range []bool{false, true} {
		dest := filepath.Join(t.TempDir(), "d")
		if existed {
			if err := os.Mkdir(dest, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		opts := okOptions("http://127.0.0.1:1", nil)
		opts.AfterGitInit = func(d string) error {
			obj := filepath.Join(d, ".git", "objects", "ab")
			if err := os.MkdirAll(obj, 0o755); err != nil {
				return err
			}
			f := filepath.Join(obj, "cdef")
			if err := os.WriteFile(f, []byte("x"), 0o444); err != nil {
				return err
			}
			if err := os.Chmod(obj, 0o555); err != nil {
				return err
			}
			return errors.New("fallo inyectado")
		}
		if err := Run(pkgRoot(t), dest, opts); err == nil {
			t.Fatal("se esperaba error")
		}
		if existed {
			if n := entries(t, dest); len(n) != 0 {
				t.Errorf("la carpeta debía quedar con 0 entradas: %v", n)
			}
		} else if _, e := os.Stat(dest); e == nil {
			t.Error("la carpeta creada debía borrarse")
		}
	}
}

func TestInstallWithoutGitIdentity(t *testing.T) {
	gitEnv(t, "")
	url, sums := fakeLint(t)
	dest := filepath.Join(t.TempDir(), "d")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	err := Run(pkgRoot(t), dest, okOptions(url, sums))
	if err == nil {
		if got := gitOut(t, dest, "rev-list", "--count", "HEAD"); got != "1" {
			t.Errorf("commits = %s, se esperaba exactamente 1", got)
		}
		return
	}
	if !strings.Contains(err.Error(), "user.name") {
		t.Errorf("el mensaje debe nombrar user.name: %v", err)
	}
	if n := entries(t, dest); len(n) != 0 {
		t.Errorf("la carpeta debía quedar con 0 entradas: %v", n)
	}
}

func TestInstallIgnoresGlobalHooks(t *testing.T) {
	hooks := t.TempDir()
	script := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	gitEnv(t, identity+"[core]\n\thooksPath = "+filepath.ToSlash(hooks)+"\n")
	url, sums := fakeLint(t)
	dest := filepath.Join(t.TempDir(), "d")
	if err := Run(pkgRoot(t), dest, okOptions(url, sums)); err != nil {
		t.Fatal(err)
	}
	if got := gitOut(t, dest, "rev-list", "--count", "HEAD"); got != "1" {
		t.Errorf("commits = %s, se esperaba 1", got)
	}
	subj := gitOut(t, dest, "log", "-1", "--format=%s")
	if !regexp.MustCompile(`^Install Aquas Light v\d+\.\d+\.\d+$`).MatchString(subj) {
		t.Errorf("asunto inesperado: %q", subj)
	}
}
