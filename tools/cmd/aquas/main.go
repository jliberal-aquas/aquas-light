// Command aquas is the Aquas Light command-line tool.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/jliberal-aquas/aquas-light/tools/internal/install"
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
	case "install":
		return runInstall(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "aquas: subcomando desconocido %q\n\n%s", args[0], usage())
		return 2
	}
}

func runInstall(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		_, _ = fmt.Fprint(stderr, "aquas: falta la ruta de la carpeta destino\n\n"+usage())
		return 2
	}
	if err := install.Preflight(args[0], install.DefaultDeps()); err != nil {
		_, _ = fmt.Fprintf(stderr, "aquas: no se puede instalar:\n%v\n", err)
		return 1
	}
	wd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aquas: no se pudo leer el directorio actual: %v\n", err)
		return 1
	}
	root, err := install.FindPackageRoot(wd)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aquas: no se puede instalar: %v\n", err)
		return 1
	}
	if err := install.Copy(root, args[0]); err != nil {
		_, _ = fmt.Fprintf(stderr, "aquas: no se puede instalar: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Archivos copiados en %s con su manifiesto. El commit inicial y la reversión ante fallos llegan en S5.\n", args[0])
	return 0
}

func usage() string {
	return "Uso: aquas <subcomando>\n\nSubcomandos:\n  install <ruta>    copia el paquete en la carpeta destino\n  version           imprime la versión de aquas\n"
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
