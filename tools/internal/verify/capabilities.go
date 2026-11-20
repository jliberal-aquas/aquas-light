package verify

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/jliberal-aquas/aquas-light/tools/internal/schema"
)

// capabilityRules applies the capability rules (S11): every feature points to
// a declared capability, no capability ID or page repeats, and every
// capability with at least one archived feature has its page.
func capabilityRules(src Source, bk *schema.Backlog, entries []entry) ([]Violation, error) {
	var out []Violation
	add := func(file, pos, format string, args ...any) {
		out = append(out, Violation{File: file, Pos: pos, Msg: fmt.Sprintf(format, args...)})
	}

	declared := map[string]bool{}
	firstByID, firstByPage := map[string]int{}, map[string]int{}
	for i, c := range bk.Capabilities {
		declared[c.ID] = true
		if j, ok := firstByID[c.ID]; ok {
			add(RootMarker, fmt.Sprintf("capabilities[%d].id", i), "el ID de capacidad %s está repetido: ya se declaró en capabilities[%d]", c.ID, j)
		} else {
			firstByID[c.ID] = i
		}
		if j, ok := firstByPage[c.Page]; ok {
			add(RootMarker, fmt.Sprintf("capabilities[%d].page", i), "la página %s está repetida: la capacidad %s usa la misma que la capacidad %s (capabilities[%d]); cada capacidad necesita la suya", c.Page, c.ID, bk.Capabilities[j].ID, j)
		} else {
			firstByPage[c.Page] = i
		}
	}

	archived := map[string]string{} // capability ID -> first archived feature
	for _, e := range entries {
		if e.typ != "feature" {
			continue
		}
		if e.capability == nil {
			add(e.file, e.pos("capability"), "la feature %s no declara capability; debe apuntar a una capacidad declarada en capabilities", e.id)
			continue
		}
		if !declared[*e.capability] {
			add(e.file, e.pos("capability"), "la feature %s apunta a la capacidad %q, que no está declarada en capabilities de %s", e.id, *e.capability, RootMarker)
			continue
		}
		if e.file != RootMarker {
			if _, ok := archived[*e.capability]; !ok {
				archived[*e.capability] = e.id
			}
		}
	}

	for i, c := range bk.Capabilities {
		feature, ok := archived[c.ID]
		if !ok {
			continue
		}
		reason, err := pageProblem(src, c.ID, c.Page)
		if err != nil {
			return nil, err
		}
		if reason != "" {
			add(RootMarker, fmt.Sprintf("capabilities[%d].page", i), "la capacidad %s tiene la feature archivada %s y su página %q no es válida: %s", c.ID, feature, c.Page, reason)
		}
	}
	return out, nil
}

// pageProblem says in Spanish why page is not the page of capability id, or
// returns "" when it is. Nothing outside the repo is ever opened: the path is
// checked as text first, then walked one folder at a time through listings,
// and the file is read only once it is known to be a regular file in the repo.
func pageProblem(src Source, id, page string) (string, error) {
	switch {
	case strings.Contains(page, `\`):
		return "usa barras invertidas; la ruta se escribe con /", nil
	case page == "" || path.IsAbs(page) || strings.Contains(page, ":"):
		return "debe ser una ruta relativa al repo, como docs/governance/" + id + "-nombre.md", nil
	case path.Clean(page) != page || strings.HasPrefix(page, "../") || strings.Contains(page, "/../"):
		return "la ruta no está normalizada o sale del repo (. o .. no se permiten)", nil
	}
	segs := strings.Split(page, "/")
	if len(segs) != 3 || segs[0] != "docs" || (segs[1] != "governance" && segs[1] != "product") {
		return "debe estar directamente en docs/governance/ o docs/product/", nil
	}
	file := segs[2]
	if name, ok := strings.CutPrefix(file, id+"-"); !ok || !strings.HasSuffix(name, ".md") || len(name) <= len(".md") {
		return "el nombre del archivo debe ser " + id + "-<nombre>.md, con el ID de esta capacidad", nil
	}

	dir := ""
	for i, seg := range segs {
		entries, err := src.Entries(dir)
		if err != nil {
			return "", fmt.Errorf("no se pudo listar %q: %w", dir, err)
		}
		var found *Entry
		var near string
		for j := range entries {
			switch {
			case entries[j].Name == seg:
				found = &entries[j]
			case strings.EqualFold(entries[j].Name, seg):
				near = entries[j].Name
			}
		}
		last := i == len(segs)-1
		switch {
		case found == nil && near != "":
			return fmt.Sprintf("no existe %q: hay %q y las mayúsculas deben coincidir exactamente", seg, near), nil
		case found == nil && last:
			return "el archivo no existe", nil
		case found == nil:
			return fmt.Sprintf("no existe la carpeta %q", seg), nil
		case found.Kind == KindLink:
			return fmt.Sprintf("%q es un enlace simbólico; debe ser un archivo real dentro del repo", seg), nil
		case last && found.Kind == KindDir:
			return "es un directorio y debe ser un archivo", nil
		case last && found.Kind != KindFile, !last && found.Kind != KindDir:
			return fmt.Sprintf("%q no es un archivo ni una carpeta normales", seg), nil
		}
		dir = path.Join(dir, seg)
	}
	data, err := src.Read(page)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "el archivo no existe", nil
		}
		return "", fmt.Errorf("no se pudo leer %s: %w", page, err)
	}
	if len(data) == 0 {
		return "el archivo está vacío (0 bytes)", nil
	}
	return "", nil
}
