// Package verify checks the governed JSON files of a repo against their
// typed shapes. Cross-file rules live in the rule files of this package.
package verify

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/jliberal-aquas/aquas-light/tools/internal/schema"
)

// Source gives the rules read access to repo files, from disk or from the
// git index. Paths use forward slashes and are relative to the repo root.
// Read returns an error wrapping fs.ErrNotExist for a missing file; List
// returns the sorted names of the .json files directly inside dir and an
// empty list when dir does not exist. Entries returns everything directly
// inside dir, with the exact stored name and the kind of each one, without
// following links; the root is the empty string and a missing dir lists empty.
type Source interface {
	Read(path string) ([]byte, error)
	List(dir string) ([]string, error)
	Entries(dir string) ([]Entry, error)
}

// Kind tells what an entry of a folder is.
type Kind int

// The kinds of entry. Anything that is not a regular file, a folder or a
// symbolic link (a junction, a submodule, a socket) is KindOther.
const (
	KindFile Kind = iota
	KindDir
	KindLink
	KindOther
)

// Entry is one name inside a folder.
type Entry struct {
	Name string
	Kind Kind
}

// Violation is one problem in one file.
type Violation struct {
	File   string
	Pos    string
	Msg    string
	offset int
}

// String formats the violation as `<file>: <position>: <message>`.
func (v Violation) String() string { return v.File + ": " + v.Pos + ": " + v.Msg }

// RootMarker is the file that marks the root of a governed repo.
const RootMarker = "backlog.json"

// FindRoot climbs from start to the folder that holds backlog.json.
func FindRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("no se pudo resolver la carpeta %s: %w", start, err)
	}
	for {
		if st, err := os.Stat(filepath.Join(dir, RootMarker)); err == nil && st.Mode().IsRegular() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no se encontró %s en %s ni en ninguna carpeta superior", RootMarker, start)
		}
		dir = parent
	}
}

type diskSource struct{ root string }

// Disk reads files from the working tree under root.
func Disk(root string) Source { return diskSource{root} }

func (d diskSource) Read(p string) ([]byte, error) {
	return os.ReadFile(filepath.Join(d.root, filepath.FromSlash(p)))
}

func (d diskSource) List(dir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(d.root, filepath.FromSlash(dir)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func (d diskSource) Entries(dir string) ([]Entry, error) {
	entries, err := os.ReadDir(filepath.Join(d.root, filepath.FromSlash(dir)))
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, e := range entries {
		k := KindOther
		switch t := e.Type(); {
		case t.IsRegular():
			k = KindFile
		case t.IsDir():
			k = KindDir
		case t&fs.ModeSymlink != 0:
			k = KindLink
		}
		out = append(out, Entry{e.Name(), k})
	}
	return out, nil
}

type indexSource struct {
	root  string
	env   []string
	files map[string]bool
	modes map[string]string
}

// Index reads files from the git index of the repo at root. It honors
// GIT_INDEX_FILE and ignores an inherited GIT_DIR and GIT_WORK_TREE, so a
// hook running inside another repo's environment still looks at root.
func Index(root string) (Source, error) {
	s := &indexSource{root: root, env: gitEnv()}
	prefix, err := s.git("rev-parse", "--show-prefix")
	if err != nil {
		if errors.Is(err, errNotRepo) {
			return nil, fmt.Errorf("%s no es un repositorio git: --staged lee el índice de git y no hay un .git en esa carpeta", root)
		}
		return nil, err
	}
	if strings.TrimSpace(string(prefix)) != "" {
		return nil, fmt.Errorf("%s no es un repositorio git propio: está dentro de otro repositorio y no tiene su propio .git", root)
	}
	out, err := s.git("ls-files", "-z", "-s")
	if err != nil {
		return nil, err
	}
	s.files = map[string]bool{}
	s.modes = map[string]string{}
	for _, rec := range strings.Split(string(out), "\x00") {
		meta, name, ok := strings.Cut(rec, "\t")
		if !ok || name == "" {
			continue
		}
		s.files[name] = true
		s.modes[name], _, _ = strings.Cut(meta, " ")
	}
	return s, nil
}

// gitEnv is the environment for git: the inherited one without GIT_DIR and
// GIT_WORK_TREE, and with GIT_INDEX_FILE made absolute.
func gitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		key, val, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(key) {
		case "GIT_DIR", "GIT_WORK_TREE":
			continue
		case "GIT_INDEX_FILE":
			if abs, err := filepath.Abs(val); err == nil && val != "" {
				kv = key + "=" + abs
			}
		}
		env = append(env, kv)
	}
	return append(env, "LC_ALL=C") // stable git messages, for the not-a-repo check
}

var errNotRepo = errors.New("no es un repositorio git")

func (s *indexSource) git(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = s.root
	cmd.Env = s.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "not a git repository") {
			return nil, errNotRepo
		}
		var notFound *exec.Error
		if errors.As(err, &notFound) {
			return nil, fmt.Errorf("no se pudo leer el índice de git: git no está instalado o no está en el PATH")
		}
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("no se pudo leer el índice de git en %s (git %s): %s", s.root, args[0], detail)
	}
	return stdout.Bytes(), nil
}

func (s *indexSource) Read(p string) ([]byte, error) {
	if !s.files[p] {
		return nil, fmt.Errorf("%s no está en el índice de git: %w", p, fs.ErrNotExist)
	}
	return s.git("cat-file", "blob", ":./"+p)
}

func (s *indexSource) List(dir string) ([]string, error) {
	var names []string
	for p := range s.files {
		rest, ok := strings.CutPrefix(p, dir+"/")
		if ok && !strings.Contains(rest, "/") && strings.HasSuffix(rest, ".json") {
			names = append(names, rest)
		}
	}
	sort.Strings(names)
	return names, nil
}

func (s *indexSource) Entries(dir string) ([]Entry, error) {
	prefix := ""
	if dir != "" {
		prefix = dir + "/"
	}
	seen := map[string]bool{}
	var out []Entry
	for p := range s.files {
		rest, ok := strings.CutPrefix(p, prefix)
		if !ok {
			continue
		}
		name, _, nested := strings.Cut(rest, "/")
		if seen[name] {
			continue
		}
		seen[name] = true
		k := KindDir
		if !nested {
			switch s.modes[p] {
			case "100644", "100755":
				k = KindFile
			case "120000":
				k = KindLink
			default:
				k = KindOther
			}
		}
		out = append(out, Entry{name, k})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// typedFile is one file to check and the Go type that describes it.
type typedFile struct {
	path     string
	target   any
	required bool
}

func specTarget(name string) (any, bool) {
	switch {
	case strings.HasPrefix(name, "I"):
		return new(schema.SpecInitiative), true
	case strings.HasPrefix(name, "F"):
		return new(schema.SpecFeature), true
	case strings.HasPrefix(name, "S"):
		return new(schema.SpecSlice), true
	case strings.HasPrefix(name, "B"):
		return new(schema.SpecBug), true
	}
	return nil, false
}

// Run checks every governed file in src and returns the violations sorted by
// file and position. The error means the check could not run (for example the
// git index could not be read); it is never a verdict on the files.
func Run(src Source) ([]Violation, error) {
	files := []typedFile{
		{RootMarker, new(schema.Backlog), true},
		{"canon/config.json", new(schema.Config), true},
	}
	var out []Violation
	archive, err := src.List("archive")
	if err != nil {
		return nil, err
	}
	for _, n := range archive {
		files = append(files, typedFile{"archive/" + n, new(schema.Archive), false})
	}
	specs, err := src.List("work/specs")
	if err != nil {
		return nil, err
	}
	for _, n := range specs {
		target, ok := specTarget(n)
		if !ok {
			out = append(out, Violation{File: "work/specs/" + n, Pos: "raíz", Msg: "nombre de spec desconocido: se esperaba I, F, S o B seguido del número, por ejemplo S9.json"})
			continue
		}
		files = append(files, typedFile{"work/specs/" + n, target, false})
	}
	requests, err := src.List("work/requests")
	if err != nil {
		return nil, err
	}
	for _, n := range requests {
		files = append(files, typedFile{"work/requests/" + n, new(schema.Request), false})
	}

	broken, missing := map[string]bool{}, map[string]bool{}
	for _, f := range files {
		data, err := src.Read(f.path)
		if errors.Is(err, fs.ErrNotExist) {
			missing[f.path] = true
			if f.required {
				out = append(out, Violation{File: f.path, Pos: "raíz", Msg: "falta el archivo"})
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("no se pudo leer %s: %w", f.path, err)
		}
		found := schema.Decode(data, f.target)
		broken[f.path] = len(found) > 0
		for _, v := range found {
			pos := v.Path
			if pos == "" {
				pos = "raíz"
			}
			out = append(out, Violation{File: f.path, Pos: pos, Msg: v.Msg, offset: v.Offset})
		}
	}
	if bk, ok := files[0].target.(*schema.Backlog); ok && !broken[RootMarker] && !missing[RootMarker] {
		archives := map[string]*schema.Archive{}
		complete := true
		for _, f := range files {
			if a, ok := f.target.(*schema.Archive); ok {
				if broken[f.path] {
					complete = false
				} else {
					archives[f.path] = a
				}
			}
		}
		rules, err := backlogRules(src, bk, archives, complete)
		if err != nil {
			return nil, err
		}
		out = append(out, rules...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].offset < out[j].offset
	})
	return out, nil
}
