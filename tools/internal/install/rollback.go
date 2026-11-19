package install

import (
	"io/fs"
	"os"
	"path/filepath"
)

// topMissing returns the topmost ancestor of abs that does not exist yet, or
// abs itself when everything above it exists.
func topMissing(abs string) string {
	top := abs
	for p := filepath.Dir(abs); p != top; p, top = filepath.Dir(p), p {
		if _, err := os.Stat(p); err == nil {
			break
		}
	}
	return top
}

// removeAll deletes path, clearing read-only bits first (Windows refuses to
// delete read-only files such as those in .git/objects).
func removeAll(path string) {
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			_ = os.Chmod(p, 0o700)
		} else {
			_ = os.Chmod(p, 0o600)
		}
		return nil
	})
	_ = os.RemoveAll(path)
}

// rollback undoes a failed install. When the install created the folder it is
// removed together with the ancestors it created; when the folder already
// existed it is emptied and kept.
func rollback(abs, top string, existed bool) {
	if !existed {
		removeAll(top)
		return
	}
	list, err := os.ReadDir(abs)
	if err != nil {
		return
	}
	for _, e := range list {
		removeAll(filepath.Join(abs, e.Name()))
	}
}
