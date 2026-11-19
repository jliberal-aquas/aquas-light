package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Violation is one problem found while decoding a file.
type Violation struct {
	Path   string // JSON path of the element, empty for the root
	Offset int    // byte offset in the file, used to order violations
	Msg    string
}

var bom = []byte{0xEF, 0xBB, 0xBF}

var integerLiteral = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

// Decode checks data against the type target points to and fills it. It
// walks the JSON tokens itself instead of letting encoding/json match keys,
// so it can reject unknown, repeated, missing and null keys, wrong kinds,
// values outside closed lists, a BOM and anything after the value. Every
// message is Spanish; no library text reaches the caller.
func Decode(data []byte, target any) []Violation {
	pv := reflect.ValueOf(target)
	if pv.Kind() != reflect.Pointer || pv.IsNil() || pv.Elem().Kind() != reflect.Struct {
		return []Violation{{Msg: "error interno: el destino debe ser un puntero a un struct"}}
	}
	w := &walker{}
	if bytes.HasPrefix(data, bom) {
		w.add(0, "", "el archivo empieza con BOM de UTF-8; guárdalo sin BOM")
		data = data[len(bom):]
		w.base = len(bom)
	}
	w.data = data
	w.dec = json.NewDecoder(bytes.NewReader(data))
	w.dec.UseNumber()

	tok, err := w.next()
	switch {
	case errors.Is(err, io.EOF):
		w.add(0, "", "el archivo está vacío")
		return w.sorted()
	case err != nil:
		w.syntax(err)
		return w.sorted()
	}
	if err := w.value("", tok, pv.Elem(), field{}); err != nil {
		w.syntax(err)
		return w.sorted()
	}
	end := int(w.dec.InputOffset())
	rest := data[end:]
	trimmed := bytes.TrimLeft(rest, " \t\r\n")
	if len(trimmed) > 0 {
		off := end + len(rest) - len(trimmed)
		w.addAt(off, "", fmt.Sprintf("hay contenido después del final del valor (línea %d)", w.line(off)))
	}
	return w.sorted()
}

type walker struct {
	dec  *json.Decoder
	data []byte
	base int // bytes stripped from the start of data (the BOM)
	vs   []Violation
}

func (w *walker) add(off int, path, msg string) {
	w.vs = append(w.vs, Violation{Path: path, Offset: off, Msg: msg})
}

func (w *walker) addAt(off int, path, msg string) { w.add(off+w.base, path, msg) }

func (w *walker) offset() int { return int(w.dec.InputOffset()) }

func (w *walker) line(off int) int {
	return bytes.Count(w.data[:min(off, len(w.data))], []byte("\n")) + 1
}

// sorted orders violations by offset; equal offsets keep their emission order.
func (w *walker) sorted() []Violation {
	sort.SliceStable(w.vs, func(i, j int) bool { return w.vs[i].Offset < w.vs[j].Offset })
	return w.vs
}

func (w *walker) next() (json.Token, error) { return w.dec.Token() }

// syntax turns a tokenizer error into a Spanish violation.
func (w *walker) syntax(err error) {
	var se *json.SyntaxError
	switch {
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		w.addAt(len(w.data), "", "el archivo termina antes de cerrar el valor JSON")
	case errors.As(err, &se):
		off := int(se.Offset)
		w.addAt(off, "", fmt.Sprintf("JSON mal formado en la línea %d: hay un carácter o una puntuación que no corresponde", w.line(max(off-1, 0))))
	default:
		w.addAt(0, "", "JSON mal formado")
	}
}

// field is the parsed `aq` tag of a struct field.
type field struct {
	name     string
	index    int
	optional bool
	nullable bool
	enum     []string
	min      *int64
}

type structInfo struct {
	fields []field
	byName map[string]int
}

var infoCache sync.Map

func infoOf(t reflect.Type) *structInfo {
	if v, ok := infoCache.Load(t); ok {
		return v.(*structInfo)
	}
	si := &structInfo{byName: map[string]int{}}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		name, _, _ := strings.Cut(sf.Tag.Get("json"), ",")
		if name == "-" || !sf.IsExported() {
			continue
		}
		if name == "" {
			name = sf.Name
		}
		f := field{name: name, index: i}
		for _, part := range strings.Split(sf.Tag.Get("aq"), ",") {
			switch {
			case part == "optional":
				f.optional = true
			case part == "nullable":
				f.nullable = true
			case strings.HasPrefix(part, "enum="):
				f.enum = strings.Split(strings.TrimPrefix(part, "enum="), "|")
			case strings.HasPrefix(part, "min="):
				if n, err := strconv.ParseInt(strings.TrimPrefix(part, "min="), 10, 64); err == nil {
					f.min = &n
				}
			}
		}
		si.byName[name] = len(si.fields)
		si.fields = append(si.fields, f)
	}
	infoCache.Store(t, si)
	return si
}

var unionType = reflect.TypeFor[Union]()

func isUnion(t reflect.Type) bool { return t.Kind() == reflect.Struct && t.Implements(unionType) }

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// label is the name a violation uses for the element at path.
func label(path string) string {
	if path == "" {
		return "el archivo"
	}
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[i+1:]
	}
	return path
}

// quoted is the label of path as it appears in a message.
func quoted(path string) string {
	if path == "" {
		return "el archivo"
	}
	return "«" + label(path) + "»"
}

// received describes a token for a message.
func received(tok json.Token) string {
	switch v := tok.(type) {
	case nil:
		return "null"
	case string:
		return strconv.Quote(v)
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	case json.Delim:
		if v == '[' {
			return "una lista"
		}
		return "un objeto"
	}
	return "un valor desconocido"
}

// describe names the JSON kind a Go type expects.
func describe(t reflect.Type) string {
	if isUnion(t) {
		var parts []string
		for i := 0; i < t.NumField(); i++ {
			parts = append(parts, describe(t.Field(i).Type))
		}
		return strings.Join(parts, " o ")
	}
	switch t.Kind() {
	case reflect.Pointer:
		return describe(t.Elem())
	case reflect.String:
		return "un texto"
	case reflect.Int, reflect.Int64:
		return "un número entero"
	case reflect.Bool:
		return "verdadero o falso"
	case reflect.Slice:
		return "una lista"
	case reflect.Struct:
		return "un objeto"
	}
	return "un valor"
}

func (w *walker) mismatch(path string, tok json.Token, t reflect.Type, start int) error {
	w.addAt(start, path, fmt.Sprintf("%s debe ser %s y trae %s", quoted(path), describe(t), received(tok)))
	return w.skip(tok, path, false)
}

// skip consumes the rest of the value whose first token was already read.
// With report set it still flags repeated keys inside objects.
func (w *walker) skip(first json.Token, path string, report bool) error {
	d, ok := first.(json.Delim)
	if !ok || (d != '{' && d != '[') {
		return nil
	}
	if d == '[' {
		for i := 0; w.dec.More(); i++ {
			tok, err := w.next()
			if err != nil {
				return err
			}
			if err := w.skip(tok, fmt.Sprintf("%s[%d]", path, i), report); err != nil {
				return err
			}
		}
		_, err := w.next()
		return err
	}
	seen := map[string]bool{}
	for w.dec.More() {
		keyOff := w.offset()
		ktok, err := w.next()
		if err != nil {
			return err
		}
		key, _ := ktok.(string)
		if report && seen[key] {
			w.addAt(keyOff, path, fmt.Sprintf("clave repetida «%s»", key))
		}
		seen[key] = true
		vtok, err := w.next()
		if err != nil {
			return err
		}
		if err := w.skip(vtok, join(path, key), report); err != nil {
			return err
		}
	}
	_, err := w.next()
	return err
}

// value decodes the value that starts with tok into dst.
func (w *walker) value(path string, tok json.Token, dst reflect.Value, f field) error {
	start := w.offset()
	t := dst.Type()
	if tok == nil {
		if f.nullable {
			return nil
		}
		w.addAt(start, path, fmt.Sprintf("%s no admite null", quoted(path)))
		return nil
	}
	switch {
	case t.Kind() == reflect.Interface:
		return w.skip(tok, path, true)
	case t.Kind() == reflect.Pointer:
		p := reflect.New(t.Elem())
		err := w.value(path, tok, p.Elem(), f)
		dst.Set(p)
		return err
	case isUnion(t):
		return w.union(path, tok, dst, f, start)
	}
	switch t.Kind() {
	case reflect.String:
		s, ok := tok.(string)
		if !ok {
			return w.mismatch(path, tok, t, start)
		}
		if len(f.enum) > 0 && !contains(f.enum, s) {
			w.addAt(start, path, fmt.Sprintf("«%s» vale %q; los valores válidos son: %s", label(path), s, strings.Join(f.enum, ", ")))
			return nil
		}
		dst.SetString(s)
	case reflect.Int, reflect.Int64:
		n, ok := tok.(json.Number)
		if !ok {
			return w.mismatch(path, tok, t, start)
		}
		lit := n.String()
		v, err := strconv.ParseInt(lit, 10, 64)
		switch {
		case !integerLiteral.MatchString(lit) || err != nil:
			w.addAt(start, path, fmt.Sprintf("«%s» debe ser un número entero escrito sin decimales ni exponente y trae %s", label(path), lit))
		case f.min != nil && v < *f.min:
			w.addAt(start, path, fmt.Sprintf("«%s» debe ser un entero de %d en adelante y trae %s", label(path), *f.min, lit))
		default:
			dst.SetInt(v)
		}
	case reflect.Bool:
		b, ok := tok.(bool)
		if !ok {
			return w.mismatch(path, tok, t, start)
		}
		dst.SetBool(b)
	case reflect.Slice:
		return w.list(path, tok, dst, f, start)
	case reflect.Struct:
		return w.object(path, tok, dst, start)
	default:
		return fmt.Errorf("tipo no soportado %s", t)
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (w *walker) list(path string, tok json.Token, dst reflect.Value, f field, start int) error {
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return w.mismatch(path, tok, dst.Type(), start)
	}
	elemField := field{enum: f.enum}
	out := reflect.MakeSlice(dst.Type(), 0, 0)
	for i := 0; w.dec.More(); i++ {
		etok, err := w.next()
		if err != nil {
			return err
		}
		elem := reflect.New(dst.Type().Elem()).Elem()
		if err := w.value(fmt.Sprintf("%s[%d]", path, i), etok, elem, elemField); err != nil {
			return err
		}
		out = reflect.Append(out, elem)
	}
	if _, err := w.next(); err != nil {
		return err
	}
	dst.Set(out)
	return nil
}

func (w *walker) union(path string, tok json.Token, dst reflect.Value, f field, start int) error {
	t := dst.Type()
	for i := 0; i < t.NumField(); i++ {
		mt := t.Field(i).Type
		if unionMatches(mt, tok) {
			p := reflect.New(mt.Elem())
			err := w.value(path, tok, p.Elem(), f)
			dst.Field(i).Set(p)
			return err
		}
	}
	return w.mismatch(path, tok, t, start)
}

func unionMatches(member reflect.Type, tok json.Token) bool {
	if member.Kind() != reflect.Pointer {
		return false
	}
	switch d := tok.(type) {
	case string:
		return member.Elem().Kind() == reflect.String
	case bool:
		return member.Elem().Kind() == reflect.Bool
	case json.Number:
		return member.Elem().Kind() == reflect.Int || member.Elem().Kind() == reflect.Int64
	case json.Delim:
		if d == '{' {
			return member.Elem().Kind() == reflect.Struct
		}
		return member.Elem().Kind() == reflect.Slice
	}
	return false
}

func (w *walker) object(path string, tok json.Token, dst reflect.Value, start int) error {
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return w.mismatch(path, tok, dst.Type(), start)
	}
	info := infoOf(dst.Type())
	seen := map[string]bool{}
	for w.dec.More() {
		keyOff := w.offset()
		ktok, err := w.next()
		if err != nil {
			return err
		}
		key, _ := ktok.(string)
		vtok, err := w.next()
		if err != nil {
			return err
		}
		switch {
		case seen[key]:
			w.addAt(keyOff, path, fmt.Sprintf("clave repetida «%s»", key))
			if err := w.skip(vtok, join(path, key), true); err != nil {
				return err
			}
		case !knownKey(info, key):
			w.addAt(keyOff, path, extraMessage(info, key))
			if err := w.skip(vtok, join(path, key), true); err != nil {
				return err
			}
		default:
			fi := info.fields[info.byName[key]]
			if err := w.value(join(path, key), vtok, dst.Field(fi.index), fi); err != nil {
				return err
			}
		}
		seen[key] = true
	}
	if _, err := w.next(); err != nil {
		return err
	}
	for _, fi := range info.fields {
		if !seen[fi.name] && !fi.optional {
			w.addAt(start, path, fmt.Sprintf("falta el campo obligatorio «%s»", fi.name))
		}
	}
	return nil
}

func knownKey(info *structInfo, key string) bool {
	_, ok := info.byName[key]
	return ok
}

// extraMessage words an unknown key, hinting when only the case differs.
func extraMessage(info *structInfo, key string) string {
	for _, fi := range info.fields {
		if strings.EqualFold(fi.name, key) {
			return fmt.Sprintf("campo sobrante «%s»: las claves distinguen mayúsculas y el campo se llama «%s»", key, fi.name)
		}
	}
	return fmt.Sprintf("campo sobrante «%s»", key)
}
