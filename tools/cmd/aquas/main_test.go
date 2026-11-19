package main

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
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
