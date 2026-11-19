// Package host detects the host system and writes work/host.json.
package host

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Host is the schema of work/host.json.
type Host struct {
	OS         string            `json:"os"`
	Arch       string            `json:"arch"`
	ShellTool  string            `json:"shell_tool"`
	PathStyle  string            `json:"path_style"`
	LineEnding string            `json:"line_ending"`
	Versions   map[string]string `json:"versions"`
	Hints      []string          `json:"hints"`
}

// Options configures Write. The zero value uses the real system.
type Options struct {
	GOOS, GOARCH string
	// Version returns the version output of a tool ("go" or "git"); an error
	// or empty result yields an empty version.
	Version func(tool string) (string, error)
}

func defaultVersion(tool string) (string, error) {
	var cmd *exec.Cmd
	switch tool {
	case "go":
		cmd = exec.Command("go", "env", "GOVERSION")
	case "git":
		cmd = exec.Command("git", "--version")
	default:
		return "", fmt.Errorf("herramienta desconocida %q", tool)
	}
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func shellTool(goos string) string {
	switch goos {
	case "windows":
		return "git-bash"
	case "darwin":
		return "zsh"
	default:
		return "bash"
	}
}

// Write reads <repoRoot>/canon/hosts.json and atomically writes
// <repoRoot>/work/host.json for the detected (or injected) system.
func Write(repoRoot string, opts Options) error {
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}
	if opts.GOARCH == "" {
		opts.GOARCH = runtime.GOARCH
	}
	if opts.Version == nil {
		opts.Version = defaultVersion
	}

	raw, err := os.ReadFile(filepath.Join(repoRoot, "canon", "hosts.json"))
	if err != nil {
		return fmt.Errorf("no se pudo leer canon/hosts.json: %v", err)
	}
	var hosts map[string]struct {
		Hints []string `json:"hints"`
	}
	if err := json.Unmarshal(raw, &hosts); err != nil {
		return fmt.Errorf("canon/hosts.json no es JSON válido: %v", err)
	}
	entry, ok := hosts[opts.GOOS]
	if !ok {
		return fmt.Errorf("el sistema %q no está en canon/hosts.json; no se escribió work/host.json", opts.GOOS)
	}

	h := Host{
		OS:         opts.GOOS,
		Arch:       opts.GOARCH,
		ShellTool:  shellTool(opts.GOOS),
		PathStyle:  "posix",
		LineEnding: "lf",
		Versions:   map[string]string{},
		Hints:      entry.Hints,
	}
	if h.Hints == nil {
		h.Hints = []string{}
	}
	if opts.GOOS == "windows" {
		h.PathStyle = "windows"
	}
	for _, tool := range []string{"go", "git"} {
		v, verr := opts.Version(tool)
		if verr != nil {
			v = ""
		}
		h.Versions[tool] = strings.TrimSpace(v)
	}

	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeAtomic(filepath.Join(repoRoot, "work"), "host.json", data)
}

func writeAtomic(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear work/: %v", err)
	}
	tmp, err := os.CreateTemp(dir, ".host-*.tmp")
	if err != nil {
		return fmt.Errorf("no se pudo escribir work/%s: %v", name, err)
	}
	tmpName := tmp.Name()
	fail := func(err error) error {
		_ = os.Remove(tmpName)
		return fmt.Errorf("no se pudo escribir work/%s: %v", name, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fail(err)
	}
	if err := tmp.Chmod(0o644); err != nil && runtime.GOOS != "windows" {
		_ = tmp.Close()
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		return fail(err)
	}
	target := filepath.Join(dir, name)
	attempts := 1
	if runtime.GOOS == "windows" {
		attempts = 20
	}
	for i := 0; ; i++ {
		err = os.Rename(tmpName, target)
		if err == nil {
			return nil
		}
		if i+1 >= attempts {
			return fail(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
