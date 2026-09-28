// Package module loads and validates module manifests (spec §6) against the
// kernel's two profiles (§1.1). It imports nothing else in this repository.
package module

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// Cap is the context block's hard limit in bytes (spec §10).
	Cap = 2048
	// Reservation is the agenda line's share of Cap, outside every module's budget.
	Reservation = 256
)

type Profile string

const (
	WorkingMemory  Profile = "working-memory"
	RatifiedRecord Profile = "ratified-record"
)

type Audience string

const (
	Self Audience = "self"
	Any  Audience = "any"
)

// Bundle is what a profile fixes. A manifest picks a profile; it cannot edit
// the bundle, which is the point of having profiles rather than flags (§1.1).
type Bundle struct {
	ModelWrites     bool
	Rendered        bool
	SearchedDefault bool
	ReviewRequired  bool
	AudienceDefault Audience
	ScopeKeys       bool
	Interviewed     bool
}

func (p Profile) Bundle() (Bundle, bool) {
	switch p {
	case WorkingMemory:
		return Bundle{ModelWrites: true, SearchedDefault: true, AudienceDefault: Self, ScopeKeys: true}, true
	case RatifiedRecord:
		return Bundle{Rendered: true, ReviewRequired: true, AudienceDefault: Self, Interviewed: true}, true
	}
	return Bundle{}, false
}

type Kind struct {
	Fields        []string `json:"fields"`
	Optional      []string `json:"optional,omitempty"`
	FreshnessDays int      `json:"freshness_days,omitempty"`
	Interview     string   `json:"interview,omitempty"`
	First         string   `json:"first,omitempty"`
	Timeless      bool     `json:"timeless,omitempty"`
	// Lenses, Draft and DueField are v1.7 keys (spec §6). The kernel validates
	// their shape and attaches no meaning to the text: how to ask belongs to
	// the module, what is due belongs to the kernel (§3.9).
	Lenses   []string `json:"lenses,omitempty"`
	Draft    string   `json:"draft,omitempty"`
	DueField string   `json:"due_field,omitempty"`
}

type Manifest struct {
	Name        string            `json:"name"`
	Version     int               `json:"version"`
	Profile     Profile           `json:"profile"`
	Priority    int               `json:"priority"`
	BudgetBytes int               `json:"budget_bytes,omitempty"`
	Audience    Audience          `json:"audience,omitempty"`
	Intro       string            `json:"intro,omitempty"`
	Kinds       map[string]Kind   `json:"kinds"`
	Summary     string            `json:"summary,omitempty"`
	Adapters    []string          `json:"adapters,omitempty"`
	Skills      []string          `json:"skills,omitempty"`
	ScopeKeys   []string          `json:"scope_keys,omitempty"`
	Layout      string            `json:"layout,omitempty"`
	LegacyTypes map[string]string `json:"legacy_types,omitempty"`
	Onboarding  []string          `json:"onboarding,omitempty"`
	Dir         string            `json:"-"`
}

// Core names the modules whose audience is pinned to self (spec §7). A
// deployment that wants another consumer to read them edits the module and
// this list, visibly.
var Core = map[string]bool{"identity": true, "telos": true, "health": true, "finance": true}

// Crossing is the one kind that exists in both profiles (spec §7): a
// preference the model wrote under a working-memory module is asked under
// the named ratified-record module's rule for the same kind. Nothing else
// crosses, which is why this is a constant and not a manifest key.
var Crossing = map[string]string{"preference": "identity"}

// Adapters is the closed set of evidence adapters this kernel implements (§8.1).
// A manifest may declare a subset; a claim may name only what its module declared.
var Adapters = []string{"tracker", "forge", "date", "manual"}

// Thread is the working-memory kind the interview leaves for a topic no enabled
// module holds, and BelongsTo its one field: the module it seems to belong in
// (§7). Both are constants because the kernel enforces one rule on them — a
// thread naming a core module holds no body — and a rule needs a fixed name.
const (
	Thread    = "thread"
	BelongsTo = "belongs_to"
)

// Reserved are the frontmatter keys the kernel composes itself (spec §5). A
// kind may declare "id" — the kernel reads Record.ID from it — and no other.
var Reserved = []string{"name", "description", "module", "kind", "id", "scope", "updated", "reviewed", "retired", "snoozes"}

type Set struct {
	Modules []Manifest
}

// Load reads <dir>/<name>/module.json for every enabled name, validates each
// manifest on its own, then the set as a whole. Every error names the module.
func Load(dir string, enabled []string) (*Set, error) {
	set := &Set{}
	seen := map[string]bool{}
	for _, name := range enabled {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		mdir := filepath.Join(dir, name)
		b, err := os.ReadFile(filepath.Join(mdir, "module.json"))
		if err != nil {
			return nil, fmt.Errorf("module %s: %w", name, err)
		}
		var m Manifest
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&m); err != nil {
			return nil, fmt.Errorf("module %s: manifest not understood: %w", name, err)
		}
		m.Dir = mdir
		if m.Name != name {
			return nil, fmt.Errorf("module %s: manifest name is %q; it must match the directory", name, m.Name)
		}
		if err := m.validate(); err != nil {
			return nil, fmt.Errorf("module %s: %w", name, err)
		}
		set.Modules = append(set.Modules, m)
	}
	sort.SliceStable(set.Modules, func(i, j int) bool {
		if set.Modules[i].Priority != set.Modules[j].Priority {
			return set.Modules[i].Priority < set.Modules[j].Priority
		}
		return set.Modules[i].Name < set.Modules[j].Name
	})
	if err := set.Validate(); err != nil {
		return nil, err
	}
	return set, nil
}

func (m *Manifest) validate() error {
	if m.Version != 1 {
		return fmt.Errorf("version %d is not supported; this kernel reads version 1", m.Version)
	}
	bundle, ok := m.Profile.Bundle()
	if !ok {
		return fmt.Errorf("profile %q is not one the kernel defines (%s, %s)", m.Profile, WorkingMemory, RatifiedRecord)
	}
	if m.Priority < 0 {
		return fmt.Errorf("priority must not be negative")
	}
	if m.Audience == "" {
		m.Audience = bundle.AudienceDefault
	}
	if m.Audience != Self && m.Audience != Any {
		return fmt.Errorf("audience %q: use self or any", m.Audience)
	}
	if Core[m.Name] && m.Audience != Self {
		return fmt.Errorf("audience %q: %s is a core module and its audience is self (spec §7)", m.Audience, m.Name)
	}
	known := map[string]bool{}
	for _, a := range Adapters {
		known[a] = true
	}
	for _, a := range m.Adapters {
		if !known[a] {
			return fmt.Errorf("adapters: %q is not one this kernel implements (%s); a new adapter is a kernel change (spec §7)", a, strings.Join(Adapters, ", "))
		}
	}
	if len(m.Kinds) == 0 {
		return fmt.Errorf("a module declares at least one kind")
	}
	for name, k := range m.Kinds {
		if err := k.validate(); err != nil {
			return fmt.Errorf("kind %s: %w", name, err)
		}
	}
	switch m.Profile {
	case WorkingMemory:
		if m.BudgetBytes != 0 {
			return fmt.Errorf("budget_bytes: a working-memory module is never in the context block")
		}
		if len(m.Onboarding) > 0 {
			return fmt.Errorf("onboarding: a working-memory module is not interviewed")
		}
		if m.Summary != "" {
			return fmt.Errorf("summary: a working-memory module is never in the context block")
		}
		if m.Intro != "" {
			return fmt.Errorf("intro: a working-memory module is not interviewed (spec §6)")
		}
		for name, k := range m.Kinds {
			if len(k.Lenses) > 0 || k.Draft != "" || k.DueField != "" {
				return fmt.Errorf("kind %s: lenses, draft and due_field belong to a ratified-record kind; a working-memory module is not interviewed (spec §6)", name)
			}
		}
		if m.Layout == "" {
			m.Layout = "kind"
		}
		if m.Layout != "free" && m.Layout != "kind" {
			return fmt.Errorf("layout %q: use free or kind", m.Layout)
		}
		for typ, kind := range m.LegacyTypes {
			if _, ok := m.Kinds[kind]; !ok {
				return fmt.Errorf("legacy_types: %q maps to kind %q, which this module does not declare", typ, kind)
			}
		}
	case RatifiedRecord:
		if m.BudgetBytes <= 0 {
			return fmt.Errorf("budget_bytes is required for a ratified-record module")
		}
		if m.Summary == "" {
			return fmt.Errorf("summary is required for a ratified-record module")
		}
		if len(m.ScopeKeys) > 0 || m.Layout != "" || len(m.LegacyTypes) > 0 {
			return fmt.Errorf("scope_keys, layout and legacy_types are working-memory keys")
		}
		for _, k := range m.Onboarding {
			kind, ok := m.Kinds[k]
			if !ok {
				return fmt.Errorf("onboarding names %q, which is not a kind of this module", k)
			}
			if kind.First == "" {
				return fmt.Errorf("onboarding names %q, which has no first question", k)
			}
		}
		for name, k := range m.Kinds {
			if n := len(k.Lenses); n != 0 && (n < 2 || n > 4) {
				return fmt.Errorf("kind %s: lenses holds %d questions; two to four (spec §6)", name, n)
			}
			if k.DueField != "" {
				declared := false
				for _, f := range append(append([]string{}, k.Fields...), k.Optional...) {
					declared = declared || f == k.DueField
				}
				if !declared {
					return fmt.Errorf("kind %s: due_field %q is not a field of this kind (spec §6)", name, k.DueField)
				}
			}
		}
	}
	return nil
}

func (k Kind) validate() error {
	if k.FreshnessDays < 0 {
		return fmt.Errorf("freshness_days must not be negative")
	}
	seen := map[string]bool{}
	for _, list := range [][]string{k.Fields, k.Optional} {
		for _, f := range list {
			if seen[f] {
				return fmt.Errorf("field %q is listed twice", f)
			}
			seen[f] = true
			for _, r := range Reserved {
				if f == r && f != "id" {
					return fmt.Errorf("field %q is a reserved record key", f)
				}
			}
		}
	}
	return nil
}

// Validate checks what only the whole set can: the ratified-record budgets
// fit under the cap less the reservation (spec §10).
func (s *Set) Validate() error {
	sum := 0
	for _, m := range s.Modules {
		if m.Profile == RatifiedRecord {
			sum += m.BudgetBytes
		}
	}
	if limit := Cap - Reservation; sum > limit {
		return fmt.Errorf("enabled budgets sum to %d bytes; the cap is %d less the %d-byte agenda reservation, so at most %d", sum, Cap, Reservation, limit)
	}
	return nil
}

func (s *Set) Module(name string) (Manifest, bool) {
	for _, m := range s.Modules {
		if m.Name == name {
			return m, true
		}
	}
	return Manifest{}, false
}

func (s *Set) Kind(module, kind string) (Kind, bool) {
	m, ok := s.Module(module)
	if !ok {
		return Kind{}, false
	}
	k, ok := m.Kinds[kind]
	return k, ok
}

func (s *Set) HasProfile(p Profile) bool {
	for _, m := range s.Modules {
		if m.Profile == p {
			return true
		}
	}
	return false
}

func (s *Set) Names() []string {
	out := make([]string, 0, len(s.Modules))
	for _, m := range s.Modules {
		out = append(out, m.Name)
	}
	return out
}

func (s *Set) Profiles() []Profile {
	seen := map[Profile]bool{}
	var out []Profile
	for _, m := range s.Modules {
		if !seen[m.Profile] {
			seen[m.Profile] = true
			out = append(out, m.Profile)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Interviewed reports whether a record of this module and kind is governed
// by a ratified-record module — the check `RuleFor`'s own doc comment warns
// a caller to make (`Bundle().Interviewed` on the returned manifest), folded
// into one predicate so the two kernel packages that need it (store's
// review gate and the server's claims/reflect tools) cannot each get it
// half right: RuleFor alone returns ok=true for a working-memory module's
// own kind too, with a zero Kind and an uninterviewed manifest.
func (s *Set) Interviewed(mod, kind string) bool {
	gov, _, ok := s.RuleFor(mod, kind)
	if !ok {
		return false
	}
	bundle, _ := gov.Profile.Bundle()
	return bundle.Interviewed
}

// RuleFor names the manifest and kind that govern a record: its own module
// and kind when that module's profile is interviewed; for a working-memory
// record whose kind is in Crossing, the ratified module it is reviewed
// under, if that module is enabled and declares the kind; otherwise the
// working-memory manifest itself with a zero Kind. A caller checks
// Bundle().Interviewed on the returned manifest to know whether the record
// is reviewed at all.
func (s *Set) RuleFor(mod, kind string) (Manifest, Kind, bool) {
	man, ok := s.Module(mod)
	if !ok {
		return Manifest{}, Kind{}, false
	}
	if bundle, _ := man.Profile.Bundle(); !bundle.Interviewed {
		if target, crosses := Crossing[kind]; crosses {
			if tm, ok := s.Module(target); ok {
				if tk, ok := tm.Kinds[kind]; ok {
					return tm, tk, true
				}
			}
		}
		return man, Kind{}, true
	}
	k, ok := man.Kinds[kind]
	return man, k, ok
}

// LegacyKind maps a pre-module `type` to the module and kind that adopts it,
// through the first working-memory module that declares it in legacy_types
// (spec §5, §6).
func (s *Set) LegacyKind(typ string) (module, kind string, ok bool) {
	for _, m := range s.Modules {
		if m.Profile != WorkingMemory {
			continue
		}
		if k, found := m.LegacyTypes[typ]; found {
			return m.Name, k, true
		}
	}
	return "", "", false
}
