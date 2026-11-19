// Command aquas is the Aquas Light command-line tool.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jliberal-aquas/aquas-light/tools/internal/host"
	"github.com/jliberal-aquas/aquas-light/tools/internal/install"
	"github.com/jliberal-aquas/aquas-light/tools/internal/update"
)

const version = "0.1.0"

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage())
		return 2
	}
	switch args[0] {
	case "install", "setup":
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "-") {
				_, _ = fmt.Fprintf(stderr, "aquas: opción desconocida %q; este subcomando no acepta opciones\n\n%s", a, usage())
				return 2
			}
		}
	}
	switch args[0] {
	case "version":
		_, _ = fmt.Fprintf(stdout, "%s\n", version)
		return 0
	case "install":
		return runInstall(args[1:], stdout, stderr)
	case "update":
		return runUpdate(args[1:], stdout, stderr)
	case "host":
		return runHost(stdout, stderr)
	case "setup":
		return runSetup(stdout, stderr)
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

func parseUpdateArgs(args []string) (string, update.Takes, error) {
	takes := update.Takes{Files: map[string]string{}}
	instance := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		val := ""
		switch {
		case a == "--take":
			if i+1 >= len(args) {
				return "", takes, fmt.Errorf("falta el valor de --take: los valores válidos son `ours` y `theirs`")
			}
			i++
			val = args[i]
		case strings.HasPrefix(a, "--take="):
			val = strings.TrimPrefix(a, "--take=")
		case strings.HasPrefix(a, "-"):
			return "", takes, fmt.Errorf("opción desconocida %q", a)
		default:
			if instance != "" {
				return "", takes, fmt.Errorf("sobra el argumento %q", a)
			}
			instance = a
			continue
		}
		if j := strings.LastIndex(val, "="); j >= 0 {
			takes.Files[strings.ReplaceAll(val[:j], "\\", "/")] = val[j+1:]
		} else {
			takes.Default = val
		}
	}
	if instance == "" {
		return "", takes, fmt.Errorf("falta la ruta de la instancia")
	}
	return instance, takes, takes.Validate()
}

func runUpdate(args []string, stdout, stderr io.Writer) int {
	instance, takes, err := parseUpdateArgs(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aquas: %v\n\n%s", err, usage())
		return 2
	}
	wd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aquas: no se pudo leer el directorio actual: %v\n", err)
		return 1
	}
	root, err := install.FindPackageRoot(wd)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aquas: no se puede actualizar: %v\n", err)
		return 1
	}
	res, err := update.Update(root, instance, version, takes)
	if err != nil {
		_, _ = fmt.Fprint(stderr, updateFailure(err))
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "Aquas Light actualizado en %s: %d archivos cambiados y %d archivos en conflicto.\n", instance, len(res.Changed), len(res.Conflicts))
	return 0
}

// updateFailure words an update error: a PartialError means the instance was
// left half updated; any other error happened before writing.
func updateFailure(err error) string {
	var pe *update.PartialError
	if errors.As(err, &pe) {
		return fmt.Sprintf("aquas: la actualización quedó incompleta: %v\nAlgunos archivos ya se actualizaron y otros no. Corrige la causa y vuelve a correr `aquas update` hasta que termine.\n", err)
	}
	return fmt.Sprintf("aquas: no se actualizó nada: %v\n", err)
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

func runSetup(stdout, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aquas: no se pudo leer el directorio actual: %v\n", err)
		return 1
	}
	root, err := install.FindPackageRoot(wd)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "aquas: no se puede preparar el clon: %v\n", err)
		return 1
	}
	if err := install.Setup(root, install.Options{Version: version}); err != nil {
		_, _ = fmt.Fprintf(stderr, "aquas: no se puede preparar el clon:\n%v\n", err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "Clon preparado: bin/aquas, golangci-lint y work/host.json listos, sin crear commits.")
	return 0
}

func usage() string {
	return "Uso: aquas <subcomando>\n\nSubcomandos:\n  install <ruta>    instala el paquete en la carpeta destino (git, binarios y commit inicial)\n  update <ruta> [--take ours|theirs] [--take <ruta>=ours|theirs]...\n                    actualiza el canon y las herramientas de la instancia; los archivos del canon editados a mano son conflicto\n  host              detecta el sistema y escribe work/host.json\n  setup             prepara un clon: compila bin/aquas, baja golangci-lint y escribe work/host.json, sin commits\n  version           imprime la versión de aquas\n"
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
