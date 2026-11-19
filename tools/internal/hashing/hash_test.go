package hashing

import (
	"regexp"
	"testing"
)

func TestManifestHashIgnoresCRLF(t *testing.T) {
	lf := []byte("a\nb\nc\n")
	crlf := []byte("a\r\nb\r\nc\r\n")
	mixed := []byte("a\r\nb\nc\r\n")
	h := Sum(lf)
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(h) {
		t.Fatalf("huella inválida: %q", h)
	}
	if Sum(crlf) != h || Sum(mixed) != h {
		t.Errorf("las huellas difieren: lf=%s crlf=%s mixed=%s", h, Sum(crlf), Sum(mixed))
	}
}

func TestHashOnlyNormalizesLineEndings(t *testing.T) {
	if Sum([]byte("a \nb\n")) == Sum([]byte("a\nb\n")) {
		t.Error("un espacio al final de línea debe cambiar la huella")
	}
	if Sum([]byte("a\rb")) == Sum([]byte("a\nb")) {
		t.Error("un CR suelto no debe normalizarse")
	}
}
