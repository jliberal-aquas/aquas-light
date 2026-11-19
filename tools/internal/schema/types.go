// Package schema holds the typed shape of every JSON file that Aquas
// governs, a strict decoder for those files, and a JSON Schema generator.
//
// Struct tags drive everything:
//
//	json:"name"   the exact key (matching is case sensitive)
//	aq:"optional" the key may be absent
//	aq:"nullable" the value may be null
//	aq:"enum=a|b" closed list of valid strings (for a []string, per element)
//	aq:"min=1"    smallest valid integer
//
// A field without `optional` is required. Optional scalars and structs use
// pointers so that absence stays distinguishable from a zero value.
package schema

import "reflect"

// Union marks a struct whose fields are alternatives: the JSON kind of the
// value (text, object or list) picks the one pointer field that gets filled.
type Union interface{ IsUnion() }

// Backlog is backlog.json.
type Backlog struct {
	SchemaVersion int           `json:"schema_version" aq:"min=1"`
	NextIDs       NextIDs       `json:"next_ids"`
	Capabilities  []Capability  `json:"capabilities"`
	Items         []BacklogItem `json:"items"`
}

// NextIDs holds the global counter of every ID letter.
type NextIDs struct {
	I int `json:"I" aq:"min=1"`
	F int `json:"F" aq:"min=1"`
	S int `json:"S" aq:"min=1"`
	B int `json:"B" aq:"min=1"`
	N int `json:"N" aq:"min=1"`
	C int `json:"C" aq:"min=1"`
}

// Capability is one entry of Backlog.Capabilities.
type Capability struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Page string `json:"page"`
}

// BacklogItem is one entry of Backlog.Items.
type BacklogItem struct {
	ID         string  `json:"id"`
	Type       string  `json:"type" aq:"enum=initiative|feature|slice|bug|need"`
	Parent     *string `json:"parent" aq:"nullable"`
	State      string  `json:"state" aq:"enum=to_refine|proposed|ready|in_progress|done"`
	Priority   int     `json:"priority" aq:"min=0"`
	Spec       *string `json:"spec" aq:"nullable"`
	Capability *string `json:"capability" aq:"optional"`
	Source     *string `json:"source" aq:"optional"`
	SpecSHA256 *string `json:"spec_sha256" aq:"optional"`
	Supersedes *string `json:"supersedes" aq:"optional"`
}

// Archive is archive/<initiative>.json.
type Archive struct {
	Initiative string        `json:"initiative"`
	Items      []ArchiveItem `json:"items"`
}

// ArchiveItem is one closed entry of Archive.Items.
type ArchiveItem struct {
	ID         string    `json:"id"`
	Type       string    `json:"type" aq:"enum=initiative|feature|slice|bug|need"`
	Parent     *string   `json:"parent" aq:"nullable"`
	State      string    `json:"state" aq:"enum=to_refine|proposed|ready|in_progress|done"`
	Priority   int       `json:"priority" aq:"min=0"`
	Spec       *string   `json:"spec" aq:"nullable"`
	Capability *string   `json:"capability" aq:"optional"`
	Source     *string   `json:"source" aq:"optional"`
	SpecSHA256 *string   `json:"spec_sha256" aq:"optional"`
	Supersedes *string   `json:"supersedes" aq:"optional"`
	ClosedAt   *string   `json:"closed_at" aq:"optional"`
	Evidence   *Evidence `json:"evidence" aq:"optional"`
}

// Evidence is the closing evidence of an archived feature.
type Evidence struct {
	IntegrationTest string  `json:"integration_test"`
	Wiki            string  `json:"wiki"`
	Tester          *string `json:"tester" aq:"optional"`
	Audit           *string `json:"audit" aq:"optional"`
	ArchitectReview *string `json:"architect_review" aq:"optional"`
	Pending         *string `json:"pending" aq:"optional"`
}

// TestObject is the object form of a technical test.
type TestObject struct {
	Name          string   `json:"name"`
	RedBeforeCode bool     `json:"red_before_code"`
	Also          []string `json:"also" aq:"optional"`
}

// TechnicalTest is either a text or a TestObject.
type TechnicalTest struct {
	Text   *string
	Object *TestObject
}

// IsUnion marks TechnicalTest as a union.
func (TechnicalTest) IsUnion() {}

// DestructiveCase is one case of a slice or bug spec.
type DestructiveCase struct {
	Case   string `json:"case"`
	Expect string `json:"expect"`
}

// SpecInitiative is work/specs/I<N>.json.
type SpecInitiative struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Goal     string   `json:"goal"`
	Source   string   `json:"source"`
	Features []string `json:"features"`
	ADR      []string `json:"adr"`
}

// Review is the architect review recorded in a feature spec.
type Review struct {
	By        string   `json:"by"`
	Date      string   `json:"date"`
	Decisions []string `json:"decisions"`
}

// SpecFeature is work/specs/F<N>.json.
type SpecFeature struct {
	ID              string   `json:"id"`
	Parent          string   `json:"parent"`
	Name            string   `json:"name"`
	Capability      string   `json:"capability"`
	Acceptance      string   `json:"acceptance"`
	IntegrationTest string   `json:"integration_test"`
	Slices          []string `json:"slices"`
	Platforms       []string `json:"platforms" aq:"enum=linux|macos|windows"`
	Review          *Review  `json:"review" aq:"optional"`
}

// SpecSlice is work/specs/S<N>.json.
type SpecSlice struct {
	ID               string            `json:"id"`
	Parent           string            `json:"parent"`
	Name             string            `json:"name"`
	What             string            `json:"what"`
	Files            []string          `json:"files"`
	FunctionalTest   string            `json:"functional_test"`
	TechnicalTest    TechnicalTest     `json:"technical_test"`
	DestructiveCases []DestructiveCase `json:"destructive_cases"`
}

// Repro is the reproduction test of a bug.
type Repro struct {
	Name string `json:"name"`
	Red  string `json:"red"`
}

// SpecBug is work/specs/B<N>.json.
type SpecBug struct {
	ID               string            `json:"id"`
	Parent           string            `json:"parent"`
	Name             string            `json:"name"`
	What             string            `json:"what"`
	Files            []string          `json:"files"`
	Repro            Repro             `json:"repro"`
	FunctionalTest   string            `json:"functional_test"`
	TechnicalTest    *TechnicalTest    `json:"technical_test" aq:"optional"`
	DestructiveCases []DestructiveCase `json:"destructive_cases"`
	PlatformsNote    *string           `json:"platforms_note" aq:"optional"`
}

// QA is one question and its answer in a refinement round.
type QA struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// Round is one refinement round of a request.
type Round struct {
	Round int  `json:"round"`
	Items []QA `json:"items"`
}

// ArchitectReview is the architect review of a request.
type ArchitectReview struct {
	Date    string `json:"date"`
	Verdict string `json:"verdict"`
	Applied string `json:"applied"`
}

// Request is work/requests/<N>.json.
type Request struct {
	ID              string           `json:"id"`
	Date            string           `json:"date"`
	Type            string           `json:"type" aq:"enum=need|error"`
	RawMessage      string           `json:"raw_message"`
	Turn            *string          `json:"turn" aq:"optional"`
	RawFile         *string          `json:"raw_file" aq:"optional"`
	Attachments     []string         `json:"attachments" aq:"optional"`
	SourceReport    *string          `json:"source_report" aq:"optional"`
	Findings        []string         `json:"findings" aq:"optional"`
	Scenario        *string          `json:"scenario" aq:"optional,nullable"`
	Rounds          []Round          `json:"rounds" aq:"optional"`
	Result          []string         `json:"result" aq:"optional,nullable"`
	ADR             []string         `json:"adr" aq:"optional,nullable"`
	Glossary        *string          `json:"glossary" aq:"optional,nullable"`
	Decisions       any              `json:"decisions" aq:"optional"`
	ArchitectReview *ArchitectReview `json:"architect_review" aq:"optional"`
}

// Config is canon/config.json.
type Config struct {
	Role         string `json:"role" aq:"enum=pack|instance"`
	DefaultModel string `json:"default_model"`
	Language     string `json:"language"`
}

// Entry names one schema and the Go type it comes from.
type Entry struct {
	Name string
	Type reflect.Type
}

// Registry lists every typed file, in the order schemas are published.
func Registry() []Entry {
	return []Entry{
		{"backlog", reflect.TypeFor[Backlog]()},
		{"archive", reflect.TypeFor[Archive]()},
		{"spec-initiative", reflect.TypeFor[SpecInitiative]()},
		{"spec-feature", reflect.TypeFor[SpecFeature]()},
		{"spec-slice", reflect.TypeFor[SpecSlice]()},
		{"spec-bug", reflect.TypeFor[SpecBug]()},
		{"request", reflect.TypeFor[Request]()},
		{"config", reflect.TypeFor[Config]()},
	}
}
