package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jliberal-aquas/aquas-light/tools/internal/update"
)

func noPanic(t *testing.T, s string) {
	t.Helper()
	if strings.Contains(s, "panic") || strings.Contains(s, "goroutine") {
		t.Errorf("la salida contiene panic o goroutine: %q", s)
	}
}

func TestVersion(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"version"}, &out, &errb)
	if code != 0 {
		t.Fatalf("código = %d, se esperaba 0", code)
	}
	s := out.String()
	if !regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(s) {
		t.Errorf("versión inválida: %q", s)
	}
	if !strings.HasSuffix(s, "\n") || strings.Count(s, "\n") != 1 || strings.Contains(s, "\r") {
		t.Errorf("debe ser una línea terminada en LF sin CR: %q", s)
	}
}

func TestVersionOutsideGitRepo(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := os.Stat(".git"); err == nil {
		t.Fatal("la carpeta temporal no debería ser un repo")
	}
	var out, errb bytes.Buffer
	if code := run([]string{"version"}, &out, &errb); code != 0 {
		t.Fatalf("código = %d, stderr=%q", code, errb.String())
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+\n$`).MatchString(out.String()) {
		t.Errorf("salida inesperada: %q", out.String())
	}
}

func TestNoArgs(t *testing.T) {
	var out, errb bytes.Buffer
	code := run(nil, &out, &errb)
	if code == 0 {
		t.Error("se esperaba código distinto de 0")
	}
	if out.Len() != 0 {
		t.Errorf("stdout debe estar vacío: %q", out.String())
	}
	if !strings.Contains(errb.String(), "Uso") {
		t.Errorf("stderr debe traer el uso en español: %q", errb.String())
	}
	noPanic(t, out.String()+errb.String())
}

func TestUnknownSubcommand(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"noexiste"}, &out, &errb)
	if code == 0 {
		t.Error("se esperaba código distinto de 0")
	}
	if !strings.Contains(errb.String(), "noexiste") {
		t.Errorf("stderr debe nombrar noexiste: %q", errb.String())
	}
	noPanic(t, out.String()+errb.String())
}

func TestGoModHasNoRequire(t *testing.T) {
	b, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`(?m)^\s*require\b`).Match(b) {
		t.Error("go.mod no debe contener require")
	}
}

func TestHostSubcommand(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/canon", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root+"/tools/sub", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/tools/go.mod", []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hosts := `{"linux":{"hints":["a"]},"darwin":{"hints":["a"]},"windows":{"hints":["a"]}}`
	if err := os.WriteFile(root+"/canon/hosts.json", []byte(hosts), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root + "/tools/sub")
	var out, errb bytes.Buffer
	if code := run([]string{"host"}, &out, &errb); code != 0 {
		t.Fatalf("código %d: %s", code, errb.String())
	}
	if _, err := os.Stat(root + "/work/host.json"); err != nil {
		t.Errorf("falta work/host.json: %v", err)
	}
	if !strings.Contains(usage(), "host") {
		t.Errorf("usage no menciona host")
	}
}

func TestHostSubcommandOutsideRepo(t *testing.T) {
	t.Chdir(t.TempDir())
	var out, errb bytes.Buffer
	if code := run([]string{"host"}, &out, &errb); code != 1 || errb.Len() == 0 {
		t.Errorf("código %d, stderr %q", code, errb.String())
	}
}

func TestUpdateInvalidTakeExits2(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"update", t.TempDir(), "--take", "mine"}, &out, &errb); code != 2 {
		t.Fatalf("código %d", code)
	}
	if !strings.Contains(errb.String(), "ours") || !strings.Contains(errb.String(), "theirs") {
		t.Fatalf("stderr = %q", errb.String())
	}
	if code := run([]string{"update", t.TempDir(), "--take", "canon/a.md=mine"}, &out, &errb); code != 2 {
		t.Fatalf("código %d", code)
	}
	if code := run([]string{"update"}, &out, &errb); code != 2 {
		t.Fatalf("código %d", code)
	}
}

func TestSetupSubcommandOutsideRepo(t *testing.T) {
	t.Chdir(t.TempDir())
	var out, errb bytes.Buffer
	if code := run([]string{"setup"}, &out, &errb); code != 1 {
		t.Fatalf("código %d, se esperaba 1", code)
	}
	if !strings.Contains(errb.String(), "aquas:") {
		t.Errorf("stderr sin mensaje: %q", errb.String())
	}
	if !strings.Contains(usage(), "setup") {
		t.Errorf("usage no menciona setup")
	}
}

func TestUpdateFailureMessages(t *testing.T) {
	partial := updateFailure(fmt.Errorf("envuelto: %w", &update.PartialError{Path: "canon/a.md", Err: errors.New("disco lleno")}))
	for _, want := range []string{"incompleta", "vuelve a correr", "canon/a.md"} {
		if !strings.Contains(partial, want) {
			t.Errorf("el mensaje parcial no contiene %q: %q", want, partial)
		}
	}
	if strings.Contains(partial, "no se actualizó nada") {
		t.Errorf("el mensaje parcial no debe decir que no se actualizó nada: %q", partial)
	}
	plain := updateFailure(errors.New("conflicto"))
	if !strings.Contains(plain, "no se actualizó nada") || strings.Contains(plain, "incompleta") {
		t.Errorf("mensaje de validación inesperado: %q", plain)
	}
}

func TestInstallRejectsFlagAsPath(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"canon", "tools"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "tools", "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(root, "tools"))
	for _, sub := range []string{"install", "update", "setup"} {
		var out, errb bytes.Buffer
		code := run([]string{sub, "--help"}, &out, &errb)
		if code == 0 {
			t.Errorf("%s --help: código 0", sub)
		}
		if !strings.Contains(errb.String(), "Uso") {
			t.Errorf("%s --help: sin mensaje de uso: %q", sub, errb.String())
		}
		for _, p := range []string{filepath.Join(root, "tools", "--help"), filepath.Join(root, "--help")} {
			if _, err := os.Stat(p); err == nil {
				t.Errorf("%s --help: se creó %s", sub, p)
			}
		}
	}
}

// writeVerifyRepo creates a folder with a valid backlog.json and config.
func writeVerifyRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"backlog.json":      `{"schema_version": 1, "next_ids": {"I": 2, "F": 2, "S": 2, "B": 1, "N": 1, "C": 2}, "capabilities": [], "items": []}` + "\n",
		"canon/config.json": `{"role": "pack", "default_model": "m", "language": "es"}` + "\n",
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestVerifyGreenAndFromSubfolder(t *testing.T) {
	root := writeVerifyRepo(t)
	sub := filepath.Join(root, "tools", "internal")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, sub} {
		t.Chdir(dir)
		var out, errb bytes.Buffer
		if code := run([]string{"verify"}, &out, &errb); code != 0 {
			t.Fatalf("desde %s: código %d, stdout=%q stderr=%q", dir, code, out.String(), errb.String())
		}
	}
}

func TestVerifyReportsViolations(t *testing.T) {
	root := writeVerifyRepo(t)
	t.Chdir(root)
	bad := strings.Replace(mustRead(t, "backlog.json"), `"items": []`, `"items": [], "color": "rojo"`, 1)
	if err := os.WriteFile("backlog.json", []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"verify"}, &out, &errb); code != 1 {
		t.Fatalf("código %d, se esperaba 1; stderr=%q", code, errb.String())
	}
	if !strings.Contains(out.String(), "backlog.json") || !strings.Contains(out.String(), "color") {
		t.Errorf("la salida debe nombrar backlog.json y color: %q", out.String())
	}
	noPanic(t, out.String()+errb.String())
}

func mustRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestVerifyWithoutBacklogIsCode3(t *testing.T) {
	t.Chdir(t.TempDir())
	var out, errb bytes.Buffer
	code := run([]string{"verify"}, &out, &errb)
	if code != 3 {
		t.Fatalf("código %d, se esperaba 3", code)
	}
	if !strings.Contains(errb.String(), "backlog.json") {
		t.Errorf("el mensaje debe nombrar backlog.json: %q", errb.String())
	}
}

func TestVerifyBadArgsIsCode2(t *testing.T) {
	for _, args := range [][]string{{"verify", "--cualquiera"}, {"verify", "sobra"}, {"verify", "--staged", "--staged2"}} {
		var out, errb bytes.Buffer
		if code := run(args, &out, &errb); code != 2 {
			t.Errorf("%v: código %d, se esperaba 2", args, code)
		}
		if out.Len() != 0 {
			t.Errorf("%v: stdout debe estar vacío: %q", args, out.String())
		}
	}
	var out, errb bytes.Buffer
	run([]string{"verify", "--cualquiera"}, &out, &errb)
	if !strings.Contains(errb.String(), "opción desconocida") || !strings.Contains(errb.String(), "Uso:") || !strings.Contains(errb.String(), "verify") {
		t.Errorf("debe decir «opción desconocida» y mostrar el uso: %q", errb.String())
	}
}

func TestVerifyStagedReadsIndex(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no está instalado")
	}
	root := writeVerifyRepo(t)
	t.Chdir(root)
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}} {
		cmd := exec.Command("git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	// Break the working tree only: the staged copy stays valid.
	if err := os.WriteFile("backlog.json", []byte("hola"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"verify", "--staged"}, &out, &errb); code != 0 {
		t.Fatalf("--staged: código %d, stdout=%q stderr=%q", code, out.String(), errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"verify"}, &out, &errb); code != 1 {
		t.Fatalf("sin --staged lee el disco y debe fallar: código %d", code)
	}
}
