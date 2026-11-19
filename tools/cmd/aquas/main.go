// Command aquas is the Aquas Light command-line tool.
package main

import (
	"fmt"
	"io"
	"os"
)

const version = "0.1.0"

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage())
		return 2
	}
	switch args[0] {
	case "version":
		_, _ = fmt.Fprintf(stdout, "%s\n", version)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "aquas: subcomando desconocido %q\n\n%s", args[0], usage())
		return 2
	}
}

func usage() string {
	return "Uso: aquas <subcomando>\n\nSubcomandos:\n  version    imprime la versión de aquas\n"
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
