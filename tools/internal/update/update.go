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
// new package, like install. Nothing is written until every conflict is
// resolved.
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

	// Step 2: mirror.
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
		if err := writeFile(dst, data, perm); err != nil {
			return res, err
		}
		res.Changed = append(res.Changed, rel)
	}
	for rel, abs := range instFiles {
		if _, ok := pkgFiles[rel]; ok || rel == manifestRel || resolution[rel] == Ours {
			continue
		}
		if err := os.Remove(abs); err != nil {
			return res, err
		}
		res.Changed = append(res.Changed, rel)
	}

	out, err := json.MarshalIndent(install.Manifest{SchemaVersion: 1, Algorithm: "sha256-lf", Version: strings.TrimPrefix(version, "v"), Files: newFiles}, "", "  ")
	if err != nil {
		return res, err
	}
	out = append(out, '\n')
	if !bytes.Equal(out, raw) {
		if err := writeFile(filepath.Join(instance, filepath.FromSlash(manifestRel)), out, 0o644); err != nil {
			return res, err
		}
		res.Changed = append(res.Changed, manifestRel)
	}

	if changed, err := updateSeals(pkgRoot, instance); err != nil {
		return res, err
	} else if changed {
		res.Changed = append(res.Changed, sealsRel)
	}
	sort.Strings(res.Changed)
	return res, nil
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

func writeFile(dst string, data []byte, perm fs.FileMode) error {
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

// updateSeals replaces only the docs/governance/ entries of the instance's
// work/seals.json with the package's. It never creates the file.
func updateSeals(pkgRoot, instance string) (bool, error) {
	dst := filepath.Join(instance, filepath.FromSlash(sealsRel))
	if _, err := os.Stat(dst); err != nil {
		return false, nil
	}
	src := filepath.Join(pkgRoot, filepath.FromSlash(sealsRel))
	if _, err := os.Stat(src); err != nil {
		return false, nil
	}
	pkgDoc, err := readJSONObject(src)
	if err != nil {
		return false, err
	}
	instDoc, err := readJSONObject(dst)
	if err != nil {
		return false, err
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
		return false, nil
	}
	out, err := json.MarshalIndent(instDoc, "", "  ")
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(dst, append(out, '\n'), 0o644)
}
