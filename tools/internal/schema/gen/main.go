// Command gen regenerates schemas/<name>.schema.json from the Go types.
//
// Run from tools/: go run ./internal/schema/gen ../schemas
package main

import (
	"fmt"
	"os"

	"github.com/jliberal-aquas/aquas-light/tools/internal/schema"
)

func main() {
	dir := "../schemas"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := schema.Write(dir); err != nil {
		fmt.Fprintf(os.Stderr, "no se pudieron generar los esquemas: %v\n", err)
		os.Exit(1)
	}
}
