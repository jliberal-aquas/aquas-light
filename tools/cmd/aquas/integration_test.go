package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/jliberal-aquas/aquas-light/tools/internal/hashing"
	"github.com/jliberal-aquas/aquas-light/tools/internal/install"
)

func itGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, b)
	}
	return strings.TrimSpace(string(b))
}

// itGitEnv isolates git from the machine config and gives it an identity.
func itGitEnv(t *testing.T) {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(cfg, []byte("[user]\n\tname = Test\n\temail = test@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "EMAIL"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
}

// itFakeLint serves a fake golangci-lint archive for the host platform, so the
// test never touches the network. It returns the server URL and checksums.
func itFakeLint(t *testing.T) (string, map[string]string) {
	t.Helper()
	name, ext := "golangci-lint", ".tar.gz"
	if runtime.GOOS == "windows" {
		name, ext = name+".exe", ".zip"
	}
	suffix := runtime.GOOS + "-" + runtime.GOARCH + ext
	content := []byte("fake binary")
	var buf bytes.Buffer
	if runtime.GOOS == "windows" {
		zw := zip.NewWriter(&buf)
		w, err := zw.Create("golangci-lint-2.5.0/" + name)
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

func itExe() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func itList(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range list {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func itRun(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

// itCopyPackage copies the package folders update reads into a new root,
// skipping tools/bin and .git, so a test can fake a newer package.
func itCopyPackage(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	for _, top := range []string{"canon", "schemas", "tools", ".githooks", "templates", "docs/governance"} {
		base := filepath.Join(src, filepath.FromSlash(top))
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
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dst, rel), data, info.Mode().Perm())
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

func itRead(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestIntegrationInstall(t *testing.T) {
	itGitEnv(t)
	pkg, err := install.FindPackageRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	url, sums := itFakeLint(t)
	opts := install.Options{Version: version, LintBaseURL: url, LintChecksums: sums}
	subject := "Install Aquas Light v" + version

	// (a) install into a new (missing) path, with a missing parent too.
	inst := filepath.Join(t.TempDir(), "parent", "instance")
	if err := install.Run(pkg, inst, opts); err != nil {
		t.Fatalf("install: %v", err)
	}
	if got := itGit(t, inst, "rev-list", "--count", "HEAD"); got != "1" {
		t.Errorf("commits = %s, want 1", got)
	}
	if got := itGit(t, inst, "log", "-1", "--format=%s"); got != subject {
		t.Errorf("subject = %q, want %q", got, subject)
	}
	if st := itGit(t, inst, "status", "--porcelain"); st != "" {
		t.Errorf("tree not clean: %q", st)
	}
	for _, f := range []string{"canon", "tools/go.mod", "tools/cmd/aquas/main.go", "bin/aquas" + itExe(), "bin/golangci-lint" + itExe(), "work/host.json", "canon/manifest.json"} {
		if _, err := os.Stat(filepath.Join(inst, filepath.FromSlash(f))); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(inst, "canon", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mf install.Manifest
	if err := json.Unmarshal(raw, &mf); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if mf.SchemaVersion != 1 || len(mf.Files) == 0 {
		t.Fatalf("manifest unexpected: %+v", mf)
	}
	if mf.Version != version {
		t.Errorf("manifest version = %q, want %q", mf.Version, version)
	}
	var canonFile string
	for p := range mf.Files {
		if strings.HasPrefix(p, "canon/") && p != "canon/manifest.json" && (canonFile == "" || p < canonFile) {
			canonFile = p
		}
	}
	if canonFile == "" {
		t.Fatal("manifest lists no canon file")
	}
	for rel, want := range mf.Files {
		if got := hashing.Sum([]byte(itRead(t, inst, rel))); got != want {
			t.Errorf("manifest hash of %s = %s, file hashes to %s", rel, want, got)
		}
	}
	_ = filepath.WalkDir(filepath.Join(inst, "canon"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(inst, p)
		rel = filepath.ToSlash(rel)
		if _, ok := mf.Files[rel]; !ok && rel != "canon/manifest.json" && d.Name() != ".gitkeep" {
			t.Errorf("%s is not in the manifest", rel)
		}
		return nil
	})
	var otherFile string
	for p := range mf.Files {
		if strings.HasPrefix(p, "canon/") && p != canonFile && (otherFile == "" || p < otherFile) {
			otherFile = p
		}
	}
	if otherFile == "" {
		t.Fatal("manifest lists fewer than two canon files")
	}
	hostRaw, err := os.ReadFile(filepath.Join(inst, "work", "host.json"))
	if err != nil {
		t.Fatal(err)
	}
	var h struct {
		OS string `json:"os"`
	}
	if err := json.Unmarshal(hostRaw, &h); err != nil || h.OS != runtime.GOOS {
		t.Errorf("host.json os = %q (err %v), want %q", h.OS, err, runtime.GOOS)
	}

	// (b) a folder with any file: nothing is created, nothing changes.
	full := filepath.Join(t.TempDir(), "full")
	if err := os.Mkdir(full, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(full, "notes.txt")
	if err := os.WriteFile(keep, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(pkg)
	code, _, stderr := itRun("install", full)
	if code != 1 || stderr == "" {
		t.Errorf("install into non-empty folder: code %d, stderr %q", code, stderr)
	}
	if got := itList(t, full); len(got) != 1 || got[0] != "notes.txt" {
		t.Errorf("folder changed: %v", got)
	}
	if b, _ := os.ReadFile(keep); string(b) != "mine\n" {
		t.Errorf("notes.txt changed: %q", b)
	}
	if err := install.Run(pkg, full, opts); err == nil {
		t.Error("install.Run into non-empty folder must fail")
	}
	if got := itList(t, full); len(got) != 1 || got[0] != "notes.txt" {
		t.Errorf("folder changed after Run: %v", got)
	}

	// (c) edit a canon file, update from the package: conflict, nothing touched.
	target := filepath.Join(inst, filepath.FromSlash(canonFile))
	if err := os.WriteFile(target, []byte("edited by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = itRun("update", inst)
	if code != 1 || !strings.Contains(stderr, canonFile) {
		t.Errorf("update with local edit: code %d, stderr %q", code, stderr)
	}
	if b, _ := os.ReadFile(target); string(b) != "edited by hand\n" {
		t.Errorf("edited file was overwritten: %q", b)
	}
	code, stdout, stderr := itRun("update", inst, "--take=theirs")
	if code != 0 {
		t.Fatalf("update --take=theirs: code %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "actualizado") {
		t.Errorf("stdout = %q", stdout)
	}
	want, err := os.ReadFile(filepath.Join(pkg, filepath.FromSlash(canonFile)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))) {
		t.Errorf("%s was not restored from the package", canonFile)
	}

	// (c2) a newer package changes one file the human edited and one they did
	// not: the edited one conflicts, the other is replaced only once resolved.
	v2 := itCopyPackage(t, pkg)
	for _, rel := range []string{canonFile, otherFile} {
		abs := filepath.Join(v2, filepath.FromSlash(rel))
		b, err := os.ReadFile(abs)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, append(b, []byte("\nchanged in v2\n")...), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	v2Other := itRead(t, v2, otherFile)
	oldOther := itRead(t, inst, otherFile)
	if err := os.WriteFile(target, []byte("edited again\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(v2)
	code, _, stderr = itRun("update", inst)
	if code != 1 || !strings.Contains(stderr, canonFile) || strings.Contains(stderr, otherFile) {
		t.Errorf("update from v2: code %d, stderr %q", code, stderr)
	}
	if itRead(t, inst, otherFile) != oldOther || itRead(t, inst, canonFile) != "edited again\n" {
		t.Error("an unresolved conflict must leave every file untouched")
	}
	code, _, stderr = itRun("update", inst, "--take="+canonFile+"=ours")
	if code != 0 {
		t.Fatalf("update --take ours: code %d, stderr %q", code, stderr)
	}
	if itRead(t, inst, otherFile) != v2Other {
		t.Errorf("unedited %s was not replaced by the new package", otherFile)
	}
	if itRead(t, inst, canonFile) != "edited again\n" {
		t.Errorf("edited %s must stay with the human version", canonFile)
	}
	raw, err = os.ReadFile(filepath.Join(inst, "canon", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mf2 install.Manifest
	if err := json.Unmarshal(raw, &mf2); err != nil {
		t.Fatal(err)
	}
	if mf2.Files[otherFile] != hashing.Sum([]byte(v2Other)) {
		t.Errorf("manifest not updated for %s", otherFile)
	}

	// (d) clone the instance and run setup.
	clone := filepath.Join(t.TempDir(), "clone")
	itGit(t, t.TempDir(), "clone", "-q", "--no-hardlinks", inst, clone)
	before := itGit(t, clone, "rev-parse", "HEAD")
	if err := install.Setup(clone, opts); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for _, f := range []string{"bin/aquas" + itExe(), "bin/golangci-lint" + itExe(), "work/host.json"} {
		if _, err := os.Stat(filepath.Join(clone, filepath.FromSlash(f))); err != nil {
			t.Errorf("clone missing %s: %v", f, err)
		}
	}
	if got := itGit(t, clone, "config", "--local", "core.hooksPath"); got != ".githooks" {
		t.Errorf("core.hooksPath = %q, want .githooks", got)
	}
	if after := itGit(t, clone, "rev-parse", "HEAD"); after != before {
		t.Errorf("setup changed HEAD: %s -> %s", before, after)
	}
	if got := itGit(t, clone, "rev-list", "--count", "HEAD"); got != "1" {
		t.Errorf("clone commits = %s, want 1", got)
	}
	if st := itGit(t, clone, "status", "--porcelain"); st != "" {
		t.Errorf("clone tree not clean: %q", st)
	}
}
