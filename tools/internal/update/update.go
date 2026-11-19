// Package update implements `aquas update`: it replaces the canon and the
// tooling of an instance with the ones carried by a newer package.
package update

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/jliberal-aquas/aquas-light/tools/internal/hashing"
	"github.com/jliberal-aquas/aquas-light/tools/internal/install"
)

const (
	// Ours keeps the local version of a conflicting file.
	Ours = "ours"
	// Theirs takes the package version of a conflicting file.
	Theirs = "theirs"
)

// mirrored are the folders replaced as a mirror of the package.
var mirrored = []string{"canon", "schemas", "tools", ".githooks", "templates", "docs/governance"}

const (
	manifestRel = "canon/manifest.json"
	sealsRel    = "work/seals.json"
	govPrefix   = "docs/governance/"
)

// Takes tells how to resolve conflicts: Default applies to every conflict
// without an entry in Files; Files maps a slash-separated path to "ours" or
// "theirs".
type Takes struct {
	Default string
	Files   map[string]string
}

// Result reports what Update did.
type Result struct {
	Changed   []string
	Conflicts []string
}

// ConflictError lists the conflicting files left unresolved.
type ConflictError struct{ Paths []string }

func (e *ConflictError) Error() string {
	return "archivos del canon editados localmente, sin resolver (use --take ours o --take theirs):\n  " + strings.Join(e.Paths, "\n  ")
}

// Validate checks that every take value is ours or theirs.
func (t Takes) Validate() error {
	check := func(label, v string) error {
		if v != "" && v != Ours && v != Theirs {
			return fmt.Errorf("valor de --take no válido %q%s: los valores válidos son `ours` y `theirs`", v, label)
		}
		return nil
	}
	if err := check("", t.Default); err != nil {
		return err
	}
	for p, v := range t.Files {
		if v == "" {
			return fmt.Errorf("falta el valor de --take para %s: los valores válidos son `ours` y `theirs`", p)
		}
		if err := check(" para "+p, v); err != nil {
			return err
		}
	}
	return nil
}

func (t Takes) resolve(rel string) string {
	if v, ok := t.Files[rel]; ok {
		return v
	}
	return t.Default
}

// Update replaces the instance with what pkgRoot carries. It runs from the
// new package, like install. Everything that can fail is validated first and
// nothing is written until then; a failure while writing is a *PartialError.
func Update(pkgRoot, instance, version string, takes Takes) (Result, error) {
	var res Result
	if err := takes.Validate(); err != nil {
		return res, err
	}
	raw, err := os.ReadFile(filepath.Join(instance, filepath.FromSlash(manifestRel)))
	if err != nil {
		return res, fmt.Errorf("%s no es una instancia de Aquas Light (no se pudo leer %s): %w", instance, manifestRel, err)
	}
	var old install.Manifest
	if err := json.Unmarshal(raw, &old); err != nil {
		return res, fmt.Errorf("%s ilegible: %w", manifestRel, err)
	}

	if err := refuseSymlinks(instance); err != nil {
		return res, err
	}

	pkgFiles := map[string]string{} // rel -> abs, for every mirrored folder
	for _, top := range mirrored {
		if err := walk(pkgRoot, top, pkgFiles); err != nil {
			return res, fmt.Errorf("no se pudo leer %s del paquete: %w", top, err)
		}
	}
	for _, need := range []string{"canon", "tools"} {
		if !hasPrefix(pkgFiles, need+"/") {
			return res, fmt.Errorf("falta la carpeta %s en el paquete", need)
		}
	}
	instFiles := map[string]string{}
	for _, top := range mirrored {
		if err := walk(instance, top, instFiles); err != nil {
			return res, fmt.Errorf("no se pudo leer %s de la instancia: %w", top, err)
		}
	}

	// Step 1: find conflicts, writing nothing.
	var unresolved []string
	resolution := map[string]string{}
	for rel, abs := range instFiles {
		if !strings.HasPrefix(rel, "canon/") || rel == manifestRel {
			continue
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return res, err
		}
		if want, ok := old.Files[rel]; ok && hashing.Sum(data) == want {
			continue
		}
		if p, ok := pkgFiles[rel]; ok {
			pd, err := os.ReadFile(p)
			if err != nil {
				return res, err
			}
			if hashing.Sum(pd) == hashing.Sum(data) {
				continue
			}
		}
		res.Conflicts = append(res.Conflicts, rel)
		if v := takes.resolve(rel); v != "" {
			resolution[rel] = v
		} else {
			unresolved = append(unresolved, rel)
		}
	}
	sort.Strings(res.Conflicts)
	if len(unresolved) > 0 {
		sort.Strings(unresolved)
		return res, &ConflictError{Paths: unresolved}
	}

	// Step 2: plan the mirror, still writing nothing.
	var ops []op
	newFiles := map[string]string{}
	for rel, abs := range pkgFiles {
		if rel == manifestRel {
			continue
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return res, err
		}
		if strings.HasPrefix(rel, "canon/") || strings.HasPrefix(rel, "tools/") {
			newFiles[rel] = hashing.Sum(data)
		}
		if resolution[rel] == Ours {
			continue
		}
		dst := filepath.Join(instance, filepath.FromSlash(rel))
		if cur, err := os.ReadFile(dst); err == nil && bytes.Equal(cur, data) {
			continue
		}
		perm := fs.FileMode(0o644)
		if info, err := os.Stat(abs); err == nil {
			perm = info.Mode().Perm()
		}
		ops = append(ops, op{rel: rel, dst: dst, data: data, perm: perm})
	}
	for rel, abs := range instFiles {
		if _, ok := pkgFiles[rel]; ok || rel == manifestRel || resolution[rel] == Ours {
			continue
		}
		ops = append(ops, op{rel: rel, dst: abs, remove: true})
	}

	out, err := json.MarshalIndent(install.Manifest{SchemaVersion: 1, Algorithm: "sha256-lf", Version: strings.TrimPrefix(version, "v"), Files: newFiles}, "", "  ")
	if err != nil {
		return res, err
	}
	out = append(out, '\n')
	if !bytes.Equal(out, raw) {
		ops = append(ops, op{rel: manifestRel, dst: filepath.Join(instance, filepath.FromSlash(manifestRel)), data: out, perm: 0o644})
	}

	sealsData, err := planSeals(pkgRoot, instance)
	if err != nil {
		return res, err
	}
	if sealsData != nil {
		ops = append(ops, op{rel: sealsRel, dst: filepath.Join(instance, filepath.FromSlash(sealsRel)), data: sealsData, perm: 0o644})
	}

	for _, o := range ops {
		if err := o.check(instance); err != nil {
			return res, err
		}
	}

	// Step 3: write. From here on a failure leaves the instance half updated.
	sort.Slice(ops, func(i, j int) bool { return ops[i].rel < ops[j].rel })
	for _, o := range ops {
		var werr error
		if o.remove {
			werr = os.Remove(o.dst)
		} else {
			werr = writeFile(o.dst, o.data, o.perm)
		}
		if werr != nil {
			sort.Strings(res.Changed)
			return res, &PartialError{Path: o.rel, Err: werr}
		}
		res.Changed = append(res.Changed, o.rel)
	}
	sort.Strings(res.Changed)
	return res, nil
}

// PartialError reports a failure while writing: some files were already
// updated and the instance is left incomplete. Running update again finishes it.
type PartialError struct {
	Path string
	Err  error
}

func (e *PartialError) Error() string {
	return fmt.Sprintf("falló la escritura de %s: %v", e.Path, e.Err)
}

func (e *PartialError) Unwrap() error { return e.Err }

// op is one planned change: write data to dst, or remove dst.
type op struct {
	rel    string
	dst    string
	data   []byte
	perm   fs.FileMode
	remove bool
}

// check fails, without changing anything, when the change cannot be made
// because of what already exists at the destination.
func (o op) check(instance string) error {
	parts := strings.Split(o.rel, "/")
	cur := instance
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("no se pudo revisar %s: %w", strings.Join(parts[:i+1], "/"), err)
		}
		last := i == len(parts)-1
		switch {
		case last && !info.Mode().IsRegular():
			return fmt.Errorf("%s existe en la instancia pero no es un archivo: muévelo o bórralo y vuelve a intentar", o.rel)
		case !last && !info.IsDir():
			return fmt.Errorf("%s existe en la instancia pero no es una carpeta: %s no se puede escribir; muévelo o bórralo y vuelve a intentar", strings.Join(parts[:i+1], "/"), o.rel)
		}
	}
	if o.remove {
		return nil
	}
	f, err := os.OpenFile(o.dst, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("%s no se puede escribir: %w", o.rel, err)
	}
	return f.Close()
}

// symlinkError names the symlink found inside the instance.
func symlinkError(rel string) error {
	return fmt.Errorf("%s es un enlace simbólico: update no escribe a través de enlaces; reemplázalo por un archivo o carpeta real y vuelve a intentar", rel)
}

// checkComponents Lstat-checks every component of rel (slash-separated)
// under root. A missing component ends the check without error.
func checkComponents(root, rel string) error {
	cur := root
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("no se pudo revisar %s: %w", strings.Join(parts[:i+1], "/"), err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return symlinkError(strings.Join(parts[:i+1], "/"))
		}
	}
	return nil
}

// refuseSymlinks fails if any path update may read, write or delete inside
// the instance is, or goes through, a symbolic link. It changes nothing.
func refuseSymlinks(instance string) error {
	for _, rel := range append([]string{manifestRel, sealsRel}, mirrored...) {
		if err := checkComponents(instance, rel); err != nil {
			return err
		}
	}
	for _, top := range mirrored {
		base := filepath.Join(instance, filepath.FromSlash(top))
		if info, err := os.Lstat(base); err != nil || !info.IsDir() {
			continue
		}
		err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, rerr := filepath.Rel(instance, p)
			if rerr != nil {
				return rerr
			}
			rel = filepath.ToSlash(rel)
			if d.IsDir() && rel == "tools/bin" {
				return filepath.SkipDir
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return symlinkError(rel)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func hasPrefix(m map[string]string, prefix string) bool {
	for k := range m {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

// walk adds every regular file under root/top to out (rel slash path -> abs).
// A missing folder is not an error. tools/bin is skipped.
func walk(root, top string, out map[string]string) error {
	base := filepath.Join(root, filepath.FromSlash(top))
	if info, err := os.Stat(base); err != nil || !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "tools/bin" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			out[rel] = p
		}
		return nil
	})
}

var writeFile = func(dst string, data []byte, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, perm)
}

// sealsMap returns the object holding the seal entries: the "seals" or
// "files" member when present, else the top-level object itself.
func sealsMap(doc map[string]any) map[string]any {
	for _, k := range []string{"seals", "files"} {
		if m, ok := doc[k].(map[string]any); ok {
			return m
		}
	}
	return doc
}

func readJSONObject(p string) (map[string]any, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s ilegible: %w", p, err)
	}
	if doc == nil {
		return nil, errors.New(p + " no es un objeto JSON")
	}
	return doc, nil
}

// planSeals computes the new content of the instance's work/seals.json: only
// the docs/governance/ entries are replaced with the package's. It returns nil
// when there is nothing to write and never creates the file. It validates the
// instance file whenever it exists.
func planSeals(pkgRoot, instance string) ([]byte, error) {
	dst := filepath.Join(instance, filepath.FromSlash(sealsRel))
	if _, err := os.Stat(dst); err != nil {
		return nil, nil
	}
	instDoc, err := readJSONObject(dst)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", sealsRel, err)
	}
	src := filepath.Join(pkgRoot, filepath.FromSlash(sealsRel))
	if _, err := os.Stat(src); err != nil {
		return nil, nil
	}
	pkgDoc, err := readJSONObject(src)
	if err != nil {
		return nil, fmt.Errorf("%s del paquete: %w", sealsRel, err)
	}
	pm, im := sealsMap(pkgDoc), sealsMap(instDoc)
	before := map[string]any{}
	for k, v := range im {
		before[k] = v
	}
	for k := range im {
		if strings.HasPrefix(k, govPrefix) {
			delete(im, k)
		}
	}
	for k, v := range pm {
		if strings.HasPrefix(k, govPrefix) {
			im[k] = v
		}
	}
	if reflect.DeepEqual(before, im) {
		return nil, nil
	}
	out, err := json.MarshalIndent(instDoc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
