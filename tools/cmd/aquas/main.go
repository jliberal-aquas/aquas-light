// Command aquas is the Aquas Light command-line tool.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/jliberal-aquas/aquas-light/tools/internal/host"
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
	case "host":
		return runHost(stdout, stderr)
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
	if err := install.Run(root, args[0], install.Options{Version: version}); err != nil {
		_, _ = fmt.Fprintf(stderr, "aquas: no se puede instalar (se revirtió todo lo creado):\n%v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Aquas Light v%s instalado en %s: archivos copiados, bin/aquas y golangci-lint listos, y un commit inicial «Install Aquas Light v%s».\n", version, args[0], version)
	return 0
}

func runHost(stdout, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aquas: no se pudo leer el directorio actual: %v\n", err)
		return 1
	}
	root, err := install.FindPackageRoot(wd)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aquas: no se puede detectar el host: %v\n", err)
		return 1
	}
	if err := host.Write(root, host.Options{}); err != nil {
		_, _ = fmt.Fprintf(stderr, "aquas: no se puede detectar el host: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "work/host.json escrito.")
	return 0
}

func usage() string {
	return "Uso: aquas <subcomando>\n\nSubcomandos:\n  install <ruta>    instala el paquete en la carpeta destino (git, binarios y commit inicial)\n  host              detecta el sistema y escribe work/host.json\n  version           imprime la versión de aquas\n"
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
