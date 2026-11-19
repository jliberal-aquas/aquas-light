package install

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// cloneOfInstance installs an instance, then clones it as another machine would.
func cloneOfInstance(t *testing.T) string {
	t.Helper()
	url, sums := fakeLint(t)
	src := filepath.Join(t.TempDir(), "src")
	if err := Run(pkgRoot(t), src, okOptions(url, sums)); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "clon")
	if out, err := git(t.TempDir(), "clone", "-q", "--no-hardlinks", src, dst); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}
	return dst
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func TestSetupPreparesClone(t *testing.T) {
	gitEnv(t, identity)
	clone := cloneOfInstance(t)
	url, sums := fakeLint(t)
	before := gitOut(t, clone, "rev-parse", "HEAD")
	for i := 1; i <= 2; i++ {
		if err := Setup(clone, okOptions(url, sums)); err != nil {
			t.Fatalf("corrida %d: %v", i, err)
		}
		if got := gitOut(t, clone, "rev-parse", "HEAD"); got != before {
			t.Errorf("corrida %d: HEAD cambió: %s -> %s", i, before, got)
		}
		if _, err := git(clone, "diff", "--quiet", "HEAD"); err != nil {
			t.Errorf("corrida %d: git diff --quiet HEAD no salió con 0: %v", i, err)
		}
	}
	for _, f := range []string{"bin/aquas" + exeSuffix(), "bin/golangci-lint" + exeSuffix()} {
		if _, err := os.Stat(filepath.Join(clone, filepath.FromSlash(f))); err != nil {
			t.Errorf("falta %s: %v", f, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(clone, "work", "host.json"))
	if err != nil {
		t.Fatalf("falta work/host.json: %v", err)
	}
	var h struct {
		OS string `json:"os"`
	}
	if err := json.Unmarshal(raw, &h); err != nil || h.OS != runtime.GOOS {
		t.Errorf("host.json os = %q (err %v), se esperaba %q", h.OS, err, runtime.GOOS)
	}
	if st := gitOut(t, clone, "status", "--porcelain"); st != "" {
		t.Errorf("el árbol debía quedar limpio: %q", st)
	}
}

func TestSetupLintServerError(t *testing.T) {
	gitEnv(t, identity)
	clone := cloneOfInstance(t)
	_, sums := fakeLint(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	before := gitOut(t, clone, "rev-parse", "HEAD")
	err := Setup(clone, okOptions(srv.URL, sums))
	if err == nil || !strings.Contains(err.Error(), "golangci-lint") {
		t.Fatalf("error = %v, se esperaba mención de golangci-lint", err)
	}
	if got := gitOut(t, clone, "rev-parse", "HEAD"); got != before {
		t.Errorf("HEAD cambió: %s -> %s", before, got)
	}
	if _, e := os.Stat(filepath.Join(clone, ".git")); e != nil {
		t.Errorf("el repo no debía borrarse: %v", e)
	}
}

func TestSetupChecksumMismatch(t *testing.T) {
	gitEnv(t, identity)
	clone := cloneOfInstance(t)
	url, sums := fakeLint(t)
	want := strings.Repeat("ab", 32)
	for k := range sums {
		sums[k] = want
	}
	err := Setup(clone, okOptions(url, sums))
	if err == nil {
		t.Fatal("se esperaba error por huella distinta")
	}
	if !strings.Contains(err.Error(), want) || !regexp.MustCompile(`recibida [0-9a-f]{64}`).MatchString(err.Error()) {
		t.Errorf("el mensaje debe nombrar esperada y recibida: %v", err)
	}
	if _, e := os.Stat(filepath.Join(clone, "bin", "golangci-lint"+exeSuffix())); e == nil {
		t.Error("el binario bajado no debía quedar en disco")
	}
}

func TestSetupSetsLocalHooksPath(t *testing.T) {
	other := filepath.ToSlash(t.TempDir())
	gitEnv(t, identity+"[core]\n\thooksPath = "+other+"\n")
	clone := cloneOfInstance(t)
	url, sums := fakeLint(t)
	if err := Setup(clone, okOptions(url, sums)); err != nil {
		t.Fatal(err)
	}
	if got := gitOut(t, clone, "config", "--local", "core.hooksPath"); got != ".githooks" {
		t.Errorf("core.hooksPath local = %q, se esperaba .githooks", got)
	}
}

func TestSetupRequiresGit(t *testing.T) {
	gitEnv(t, identity)
	dir := t.TempDir()
	url, sums := fakeLint(t)
	err := Setup(dir, okOptions(url, sums))
	if err == nil {
		t.Fatal("se esperaba error sin .git")
	}
	if !strings.Contains(err.Error(), ".git") {
		t.Errorf("el mensaje debe explicar que falta .git: %v", err)
	}
	if _, e := os.Stat(filepath.Join(dir, ".git")); e == nil {
		t.Error("no debía crearse .git")
	}
}
