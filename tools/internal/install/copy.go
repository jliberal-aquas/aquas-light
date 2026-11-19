package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jliberal-aquas/aquas-light/tools/internal/hashing"
)

// FindPackageRoot walks up from start until it finds a folder holding both
// canon/ and tools/go.mod.
func FindPackageRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if isDir(filepath.Join(dir, "canon")) && isFile(filepath.Join(dir, "tools", "go.mod")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no se encontró la raíz del paquete Aquas Light (una carpeta con canon/ y tools/go.mod) subiendo desde " + start)
		}
		dir = parent
	}
}

func isDir(p string) bool {
	i, err := os.Stat(p)
	return err == nil && i.IsDir()
}

func isFile(p string) bool {
	i, err := os.Stat(p)
	return err == nil && i.Mode().IsRegular()
}

// Manifest is the content of canon/manifest.json.
type Manifest struct {
	SchemaVersion int    `json:"schema_version"`
	Algorithm     string `json:"algorithm"`
	// Version is the package version without the leading "v", such as "0.1.0".
	// Manifests written before it existed lack it and are still read.
	Version string            `json:"version,omitempty"`
	Files   map[string]string `json:"files"`
}

// Copy installs the package found at root into dest, which must already have
// passed Preflight. It copies canon/, schemas/ (if present) and tools/ byte by
// byte, records version in the manifest, copies templates/ into the root of dest, creates the starter folders and folders, and writes canon/manifest.json.
func Copy(root, dest, version string) error {
	files := map[string]string{}
	for _, top := range []string{"canon", "schemas", "tools"} {
		src := filepath.Join(root, top)
		if !isDir(src) {
			if top == "schemas" {
				continue
			}
			return fmt.Errorf("falta la carpeta %s en el paquete", src)
		}
		err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			if d.IsDir() {
				if rel == filepath.Join("tools", "bin") {
					return filepath.SkipDir
				}
				return os.MkdirAll(filepath.Join(dest, rel), 0o755)
			}
			if !d.Type().IsRegular() {
				return nil
			}
			data, err := copyFile(p, filepath.Join(dest, rel), d)
			if err != nil {
				return err
			}
			if rel != filepath.Join("canon", "manifest.json") {
				files[filepath.ToSlash(rel)] = hashing.Sum(data)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("no se pudo copiar %s: %w", top, err)
		}
	}

	if err := writeStarter(root, dest); err != nil {
		return err
	}
	return writeManifest(dest, version, files)
}

func copyFile(src, dst string, d fs.DirEntry) ([]byte, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	perm := fs.FileMode(0o644)
	if info, e := d.Info(); e == nil {
		perm = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, err
	}
	return data, os.WriteFile(dst, data, perm)
}

// dotNames maps template names without a leading dot to their final name.
var dotNames = map[string]string{"gitignore": ".gitignore", "gitattributes": ".gitattributes"}

func writeStarter(root, dest string) error {
	tdir := filepath.Join(root, "templates")
	list, err := os.ReadDir(tdir)
	if err != nil {
		return fmt.Errorf("no se pudieron leer las plantillas en %s: %w", tdir, err)
	}
	for _, e := range list {
		if !e.Type().IsRegular() {
			continue
		}
		name := e.Name()
		if dot, ok := dotNames[name]; ok {
			name = dot
		}
		if _, err := copyFile(filepath.Join(tdir, e.Name()), filepath.Join(dest, name), e); err != nil {
			return fmt.Errorf("no se pudo crear %s: %w", name, err)
		}
	}
	for _, dir := range []string{"work", "archive", "product", "docs"} {
		if err := os.MkdirAll(filepath.Join(dest, dir), 0o755); err != nil {
			return fmt.Errorf("no se pudo crear %s: %w", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dest, dir, ".gitkeep"), nil, 0o644); err != nil {
			return fmt.Errorf("no se pudo crear %s/.gitkeep: %w", dir, err)
		}
	}
	return nil
}

func writeManifest(dest, version string, files map[string]string) error {
	// json.Marshal sorts map keys, so the output is deterministic.
	out, err := json.MarshalIndent(Manifest{SchemaVersion: 1, Algorithm: "sha256-lf", Version: strings.TrimPrefix(version, "v"), Files: files}, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if err := os.WriteFile(filepath.Join(dest, "canon", "manifest.json"), out, 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir canon/manifest.json: %w", err)
	}
	return nil
}
