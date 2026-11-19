package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const hostsJSON = `{
  "linux": {"hints": ["l1", "l2"]},
  "darwin": {"hints": ["d1"]},
  "windows": {"hints": ["w1", "w2", "w3"]}
}`

func newRepo(t *testing.T, hosts string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "canon"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "canon", "hosts.json"), []byte(hosts), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func fakeVersion(tool string) (string, error) {
	if tool == "go" {
		return "go1.24.7", nil
	}
	return "", os.ErrNotExist
}

func readHost(t *testing.T, root string) Host {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "work", "host.json"))
	if err != nil {
		t.Fatal(err)
	}
	var h Host
	if err := json.Unmarshal(b, &h); err != nil {
		t.Fatalf("host.json no es JSON válido: %v\n%s", err, b)
	}
	return h
}

func TestDetectHostWritesHints(t *testing.T) {
	root := newRepo(t, hostsJSON)
	if err := Write(root, Options{GOOS: "windows", GOARCH: "amd64", Version: fakeVersion}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	h := readHost(t, root)
	if h.OS != "windows" || h.Arch != "amd64" || h.ShellTool != "git-bash" || h.PathStyle != "windows" || h.LineEnding != "lf" {
		t.Errorf("host inesperado: %+v", h)
	}
	if strings.Join(h.Hints, "|") != "w1|w2|w3" {
		t.Errorf("hints = %v", h.Hints)
	}
	if h.Versions["go"] != "go1.24.7" || h.Versions["git"] != "" {
		t.Errorf("versions = %v", h.Versions)
	}
	if _, ok := h.Versions["git"]; !ok {
		t.Errorf("falta la clave git en versions")
	}
}

func TestShellAndPathStyle(t *testing.T) {
	for goos, want := range map[string][2]string{"linux": {"bash", "posix"}, "darwin": {"zsh", "posix"}} {
		root := newRepo(t, hostsJSON)
		if err := Write(root, Options{GOOS: goos, GOARCH: "arm64", Version: fakeVersion}); err != nil {
			t.Fatal(err)
		}
		h := readHost(t, root)
		if h.ShellTool != want[0] || h.PathStyle != want[1] {
			t.Errorf("%s: %+v", goos, h)
		}
	}
}

func TestUnknownOSKeepsPreviousHost(t *testing.T) {
	root := newRepo(t, hostsJSON)
	prev := []byte("contenido previo\n")
	if err := os.MkdirAll(filepath.Join(root, "work"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, "work", "host.json")
	if err := os.WriteFile(p, prev, 0o644); err != nil {
		t.Fatal(err)
	}
	err := Write(root, Options{GOOS: "plan9", GOARCH: "386", Version: fakeVersion})
	if err == nil || !strings.Contains(err.Error(), "plan9") {
		t.Fatalf("error = %v", err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != string(prev) {
		t.Errorf("host.json previo cambió: %q", got)
	}
}

func TestNonJSONPreviousIsReplaced(t *testing.T) {
	root := newRepo(t, hostsJSON)
	if err := os.MkdirAll(filepath.Join(root, "work"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "work", "host.json"), []byte("no soy json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, Options{GOOS: "linux", GOARCH: "amd64", Version: fakeVersion}); err != nil {
		t.Fatal(err)
	}
	if h := readHost(t, root); h.OS != "linux" || len(h.Hints) != 2 {
		t.Errorf("host = %+v", h)
	}
}

func TestConcurrentWrites(t *testing.T) {
	root := newRepo(t, hostsJSON)
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- Write(root, Options{GOOS: "windows", GOARCH: "amd64", Version: fakeVersion})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("Write concurrente: %v", err)
		}
	}
	if h := readHost(t, root); h.OS != "windows" || len(h.Hints) != 3 {
		t.Errorf("host = %+v", h)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "work"))
	for _, e := range entries {
		if e.Name() != "host.json" {
			t.Errorf("sobró un temporal: %s", e.Name())
		}
	}
}

func TestBrokenHostsJSON(t *testing.T) {
	root := newRepo(t, `{"linux": `)
	err := Write(root, Options{GOOS: "linux", GOARCH: "amd64", Version: fakeVersion})
	if err == nil || !strings.Contains(err.Error(), "canon/hosts.json") {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "work", "host.json")); statErr == nil {
		t.Errorf("se escribió un host.json parcial")
	}
	entries, _ := os.ReadDir(filepath.Join(root, "work"))
	if len(entries) != 0 {
		t.Errorf("work/ no está vacío: %v", entries)
	}
}
