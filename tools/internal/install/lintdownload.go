package install

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"
)

// lintVersion is the pinned golangci-lint version (keep in sync with
// canon/toolchain.json, field golangci_lint_version).
const lintVersion = "2.5.0"

const defaultLintBaseURL = "https://github.com/golangci/golangci-lint/releases/download"

// pinnedLintSHA256 holds the official SHA-256 checksums of golangci-lint 2.5.0.
// Source: https://github.com/golangci/golangci-lint/releases/download/v2.5.0/golangci-lint-2.5.0-checksums.txt
// Keys are the asset suffix after "golangci-lint-2.5.0-".
var pinnedLintSHA256 = map[string]string{
	"darwin-amd64.tar.gz": "a7e684872b00637d642d088dde783c1b871161a92678fcf13d07abe6b5c32e36",
	"darwin-arm64.tar.gz": "0b3cbdc2a2472f60b538ebccb1b2e1ae5d938a051c010591aa68c6efd3706672",
	"linux-amd64.tar.gz":  "c77313a77e19b06123962c411d9943cc0d092bbec76b956104d18964e274902e",
	"linux-arm64.tar.gz":  "48693a98a7f4556d1117300aae240d0fe483df8d6f36dfaba56504626101a66e",
	"windows-amd64.zip":   "8d37563c2549e38135eac46e778164d2c5b1e96b9211f2087814d74ca0f358a8",
	"windows-arm64.zip":   "b08f2ac149428307a14d733772720c84893d17e0d1ff2506623b00e7246229b1",
}

const maxLintArchive = 200 << 20

// lintSuffix returns the asset key for a platform, such as "linux-amd64.tar.gz".
func lintSuffix(goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return goos + "-" + goarch + ext
}

func downloadLint(dest string, opts Options) error {
	suffix := lintSuffix(opts.GOOS, opts.GOARCH)
	want, ok := opts.LintChecksums[suffix]
	if !ok {
		return fmt.Errorf("no hay golangci-lint fijado para %s/%s", opts.GOOS, opts.GOARCH)
	}
	url := fmt.Sprintf("%s/v%s/golangci-lint-%s-%s", opts.LintBaseURL, lintVersion, lintVersion, suffix)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("no se pudo descargar golangci-lint desde %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("la descarga de golangci-lint desde %s respondió %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxLintArchive+1))
	if err != nil {
		return fmt.Errorf("se cortó la descarga de golangci-lint: %v", err)
	}
	if len(data) > maxLintArchive {
		return errors.New("el archivo de golangci-lint excede el tamaño máximo permitido")
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return fmt.Errorf("la huella SHA-256 de golangci-lint no coincide: esperada %s, recibida %s", want, got)
	}

	name := "golangci-lint"
	if opts.GOOS == "windows" {
		name += ".exe"
	}
	bin, err := extractFile(data, suffix, name)
	if err != nil {
		return err
	}
	out := filepath.Join(dest, "bin", name)
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return fmt.Errorf("no se pudo crear bin/: %v", err)
	}
	if err := os.WriteFile(out, bin, 0o755); err != nil {
		return fmt.Errorf("no se pudo escribir bin/%s: %v", name, err)
	}
	return nil
}

// extractFile returns the contents of the first archive entry named name.
func extractFile(data []byte, suffix, name string) ([]byte, error) {
	notFound := fmt.Errorf("el archivo de golangci-lint no contiene %s", name)
	if path.Ext(suffix) == ".zip" {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("archivo zip de golangci-lint inválido: %v", err)
		}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() || path.Base(f.Name) != name {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer func() { _ = rc.Close() }()
			return io.ReadAll(io.LimitReader(rc, maxLintArchive))
		}
		return nil, notFound
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("archivo tar.gz de golangci-lint inválido: %v", err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, notFound
		}
		if err != nil {
			return nil, fmt.Errorf("archivo tar.gz de golangci-lint inválido: %v", err)
		}
		if h.Typeflag == tar.TypeReg && path.Base(h.Name) == name {
			return io.ReadAll(io.LimitReader(tr, maxLintArchive))
		}
	}
}
