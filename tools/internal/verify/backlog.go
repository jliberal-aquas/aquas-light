package verify

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jliberal-aquas/aquas-light/tools/internal/hashing"
	"github.com/jliberal-aquas/aquas-light/tools/internal/schema"
)

var (
	idPattern     = regexp.MustCompile(`^[IFSBNC][1-9][0-9]*$`)
	sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// letterTypes maps the letter of an ID to the item type it names.
var letterTypes = map[byte]string{'I': "initiative", 'F': "feature", 'S': "slice", 'B': "bug", 'N': "need"}

// typeNames is how a type reads in a message.
var typeNames = map[string]string{
	"initiative": "una iniciativa", "feature": "una feature", "slice": "un slice", "bug": "un bug", "need": "una necesidad",
}

// entry is one backlog or archive item, wherever it lives.
type entry struct {
	file       string
	index      int
	id         string
	typ        string
	parent     *string
	state      string
	spec       *string
	source     *string
	specSHA    *string
	supersedes *string
	valid      bool // the ID is well formed and its letter matches the type
}

func (e entry) pos(field string) string {
	p := fmt.Sprintf("items[%d]", e.index)
	if field != "" {
		p += "." + field
	}
	return p
}

func (e entry) where() string { return e.file + " " + e.pos("") }

func fromBacklog(items []schema.BacklogItem) []entry {
	var out []entry
	for i, b := range items {
		out = append(out, entry{file: RootMarker, index: i, id: b.ID, typ: b.Type, parent: b.Parent, state: b.State,
			spec: b.Spec, source: b.Source, specSHA: b.SpecSHA256, supersedes: b.Supersedes})
	}
	return out
}

func fromArchive(file string, items []schema.ArchiveItem) []entry {
	var out []entry
	for i, b := range items {
		out = append(out, entry{file: file, index: i, id: b.ID, typ: b.Type, parent: b.Parent, state: b.State,
			spec: b.Spec, source: b.Source, specSHA: b.SpecSHA256, supersedes: b.Supersedes})
	}
	return out
}

// parseID splits a well formed ID into its letter and number. The error
// says in Spanish why the ID is not valid.
func parseID(id string) (byte, int, error) {
	if !idPattern.MatchString(id) {
		return 0, 0, errors.New("debe ser una letra I, F, S, B, N o C seguida de un entero positivo sin ceros a la izquierda")
	}
	n, err := strconv.Atoi(id[1:])
	if err != nil {
		return 0, 0, errors.New("el número es demasiado grande")
	}
	return id[0], n, nil
}

// backlogRules applies the backlog rules (S10) to the decoded backlog and
// archives. complete is false when an archive could not be read: then a
// missing ID proves nothing and is not reported.
func backlogRules(src Source, bk *schema.Backlog, archives map[string]*schema.Archive, complete bool) ([]Violation, error) {
	var out []Violation
	add := func(file, pos, format string, args ...any) {
		out = append(out, Violation{File: file, Pos: pos, Msg: fmt.Sprintf(format, args...)})
	}

	entries := fromBacklog(bk.Items)
	names := make([]string, 0, len(archives))
	for n := range archives {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		entries = append(entries, fromArchive(n, archives[n].Items)...)
	}

	// Rule 1: ID format and letter against type.
	maxByLetter := map[byte]struct {
		n  int
		id string
		at string
	}{}
	note := func(letter byte, n int, id, at string) {
		if cur, ok := maxByLetter[letter]; !ok || n > cur.n {
			maxByLetter[letter] = struct {
				n  int
				id string
				at string
			}{n, id, at}
		}
	}
	for i := range entries {
		e := &entries[i]
		letter, n, err := parseID(e.id)
		if err != nil {
			add(e.file, e.pos("id"), "el ID %q no es válido: %v", e.id, err)
			continue
		}
		note(letter, n, e.id, e.file)
		if want := letterTypes[letter]; want != e.typ {
			if want == "" {
				add(e.file, e.pos("id"), "el ID %s usa la letra C, que es de las capacidades, y no puede ser de un elemento de tipo %s", e.id, e.typ)
			} else {
				add(e.file, e.pos("id"), "el ID %s no coincide con su tipo: la letra %c es de tipo %s y el elemento declara type %s", e.id, letter, want, e.typ)
			}
			continue
		}
		e.valid = true
	}
	for i, c := range bk.Capabilities {
		letter, n, err := parseID(c.ID)
		if err != nil || letter != 'C' {
			add(RootMarker, fmt.Sprintf("capabilities[%d].id", i), "el ID de capacidad %q no es válido: debe ser C seguida de un entero positivo sin ceros a la izquierda", c.ID)
			continue
		}
		note('C', n, c.ID, RootMarker)
	}

	// Rule 2: an ID appears once across backlog.json and archive/*.json.
	var order []string
	locs := map[string][]entry{}
	for _, e := range entries {
		if !e.valid {
			continue
		}
		if _, ok := locs[e.id]; !ok {
			order = append(order, e.id)
		}
		locs[e.id] = append(locs[e.id], e)
	}
	for _, id := range order {
		l := locs[id]
		if len(l) < 2 {
			continue
		}
		var at []string
		for _, e := range l {
			at = append(at, e.where())
		}
		add(l[0].file, l[0].pos("id"), "el ID %s está repetido: aparece en %s", id, strings.Join(at, " y en "))
	}

	// Rule 3: every counter is above every ID of its letter.
	counters := []struct {
		letter byte
		value  int
	}{{'I', bk.NextIDs.I}, {'F', bk.NextIDs.F}, {'S', bk.NextIDs.S}, {'B', bk.NextIDs.B}, {'N', bk.NextIDs.N}, {'C', bk.NextIDs.C}}
	for _, c := range counters {
		if m, ok := maxByLetter[c.letter]; ok && c.value <= m.n {
			add(RootMarker, "next_ids."+string(c.letter), "el contador next_ids.%c vale %d pero existe el ID %s (en %s); debe valer más que %d", c.letter, c.value, m.id, m.at, m.n)
		}
	}

	// Lookup of well formed items, the first occurrence of each ID.
	byID := map[string]entry{}
	for _, id := range order {
		byID[id] = locs[id][0]
	}

	// Rule 4: parents.
	for _, e := range entries {
		if !e.valid {
			continue
		}
		parentID := ""
		if e.parent != nil {
			parentID = *e.parent
		}
		if e.typ == "need" && e.parent == nil {
			continue
		}
		var wantType, wantText string
		switch e.typ {
		case "initiative":
			if e.parent != nil {
				add(e.file, e.pos("parent"), "la iniciativa %s tiene padre %s; una iniciativa debe tener padre null", e.id, parentID)
			}
			continue
		case "feature":
			wantType, wantText = "initiative", "una iniciativa"
		case "slice":
			wantType, wantText = "feature", "una feature"
		case "bug":
			wantType, wantText = "slice", "un slice en estado done"
		case "need":
			if _, ok := byID[parentID]; !ok && complete {
				add(e.file, e.pos("parent"), "la necesidad %s tiene padre %s, que no existe en backlog.json ni en archive/; el padre debe ser null o un ID existente", e.id, parentID)
			}
			continue
		}
		if e.parent == nil {
			add(e.file, e.pos("parent"), "%s %s no tiene padre (null); su padre debe ser %s", typeNames[e.typ], e.id, wantText)
			continue
		}
		p, ok := byID[parentID]
		switch {
		case !ok:
			if complete {
				add(e.file, e.pos("parent"), "%s %s tiene padre %s, que no existe en backlog.json ni en archive/; su padre debe ser %s", typeNames[e.typ], e.id, parentID, wantText)
			}
		case p.typ != wantType:
			add(e.file, e.pos("parent"), "%s %s tiene padre %s, que es %s; su padre debe ser %s", typeNames[e.typ], e.id, parentID, typeNames[p.typ], wantText)
		case e.typ == "bug" && p.state != "done":
			add(e.file, e.pos("parent"), "el bug %s tiene padre %s, que está en estado %s; su padre debe ser %s", e.id, parentID, p.state, wantText)
		}
	}

	// Rule 5: at most one slice or bug in progress.
	var running []entry
	for _, e := range entries {
		if e.valid && (e.typ == "slice" || e.typ == "bug") && e.state == "in_progress" {
			running = append(running, e)
		}
	}
	if len(running) > 1 {
		var ids []string
		for _, e := range running {
			ids = append(ids, e.id)
		}
		add(running[0].file, running[0].pos("state"), "hay %d slices o bugs en in_progress (%s); solo puede haber uno", len(running), strings.Join(ids, ", "))
	}

	// Rule 6: a need to refine points to a request file that exists.
	requests, err := src.List("work/requests")
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, n := range requests {
		have["work/requests/"+n] = true
	}
	for _, e := range entries {
		if !e.valid || e.typ != "need" || e.state != "to_refine" {
			continue
		}
		switch {
		case e.source == nil || *e.source == "":
			add(e.file, e.pos("source"), "la necesidad %s está en to_refine y no tiene source; debe apuntar a un archivo de work/requests/", e.id)
		case path.Clean(*e.source) != *e.source || !strings.HasPrefix(*e.source, "work/requests/") || strings.Count(*e.source, "/") != 2:
			add(e.file, e.pos("source"), "el source %q de la necesidad %s no es válido: debe ser work/requests/<archivo>.json, sin rutas relativas", *e.source, e.id)
		case !have[*e.source]:
			add(e.file, e.pos("source"), "el source %s de la necesidad %s no existe", *e.source, e.id)
		}
	}

	// Rule 7: supersedes is only for slices and points to a done slice.
	for _, e := range entries {
		if !e.valid || e.supersedes == nil {
			continue
		}
		target := *e.supersedes
		if e.typ != "slice" {
			add(e.file, e.pos("supersedes"), "%s %s declara supersedes %s; solo un slice puede llevar ese campo", typeNames[e.typ], e.id, target)
			continue
		}
		t, ok := byID[target]
		switch {
		case target == e.id:
			add(e.file, e.pos("supersedes"), "el slice %s declara supersedes hacia sí mismo", e.id)
		case !ok:
			if complete {
				add(e.file, e.pos("supersedes"), "el slice %s declara supersedes %s, que no existe en backlog.json ni en archive/", e.id, target)
			}
		case t.typ != "slice":
			add(e.file, e.pos("supersedes"), "el slice %s declara supersedes %s, que es %s; debe apuntar a un slice done", e.id, target, typeNames[t.typ])
		case t.state != "done":
			add(e.file, e.pos("supersedes"), "el slice %s declara supersedes %s, que está en estado %s; debe apuntar a un slice done", e.id, target, t.state)
		}
	}

	// Rule 8: fingerprints of done slices and bugs.
	for _, e := range entries {
		if !e.valid {
			continue
		}
		wants := (e.typ == "slice" || e.typ == "bug") && e.state == "done"
		if !wants {
			if e.specSHA != nil {
				add(e.file, e.pos("spec_sha256"), "%s %s lleva spec_sha256 y no debe: solo lo llevan los slices y bugs en estado done (este es %s en estado %s)", typeNames[e.typ], e.id, e.typ, e.state)
			}
			continue
		}
		if e.specSHA == nil {
			add(e.file, e.pos("spec_sha256"), "%s %s está done y le falta spec_sha256", typeNames[e.typ], e.id)
			continue
		}
		if !sha256Pattern.MatchString(*e.specSHA) {
			add(e.file, e.pos("spec_sha256"), "el spec_sha256 %q de %s no es válido: deben ser 64 caracteres hexadecimales en minúscula", *e.specSHA, e.id)
			continue
		}
		if e.spec == nil || path.Clean(*e.spec) != *e.spec || !strings.HasPrefix(*e.spec, "work/specs/") {
			add(e.file, e.pos("spec"), "%s %s está done y su campo spec no apunta a un archivo de work/specs/, así que no se puede comprobar spec_sha256", typeNames[e.typ], e.id)
			continue
		}
		data, err := src.Read(*e.spec)
		if errors.Is(err, fs.ErrNotExist) {
			add(e.file, e.pos("spec"), "%s %s está done y su spec %s no existe, así que no se puede comprobar spec_sha256", typeNames[e.typ], e.id, *e.spec)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("no se pudo leer %s: %w", *e.spec, err)
		}
		if got := hashing.Sum(data); got != *e.specSHA {
			add(e.file, e.pos("spec_sha256"), "el spec_sha256 de %s (%s) no coincide con la huella actual de %s (%s); la spec cambió después de cerrarse", e.id, *e.specSHA, *e.spec, got)
		}
	}
	return out, nil
}
