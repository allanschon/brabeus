package view

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark"

	"github.com/allanschon/brabeus/internal/agenda"
	"github.com/allanschon/brabeus/internal/block"
	"github.com/allanschon/brabeus/internal/instructions"
	"github.com/allanschon/brabeus/internal/module"
)

//go:embed templates/*.html.tmpl
var kernelTemplates embed.FS

//go:embed static/view.css
var stylesheet []byte

//go:embed static/fonts/*
var fonts embed.FS

// Stylesheet is the kernel's one stylesheet, served same-origin (spec §11).
func Stylesheet() []byte { return stylesheet }

// Font returns a self-hosted font by its exact file name. It takes a name,
// never a path, and answers only for a .woff2 file, so it cannot be used to
// read anything else that is embedded.
func Font(name string) ([]byte, bool) {
	if !strings.HasSuffix(name, ".woff2") || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return nil, false
	}
	b, err := fs.ReadFile(fonts, "static/fonts/"+name)
	if err != nil {
		return nil, false
	}
	return b, true
}

// Reserved are the names the kernel's templates define. A module template
// that defines one would replace the kernel's partial without saying so
// (spec §6). The list is read from the templates themselves, so a partial
// added later is reserved without anyone listing it.
var Reserved = reservedNames()

func reservedNames() []string {
	t, err := parseKernel()
	if err != nil {
		panic(fmt.Sprintf("view: kernel templates: %v", err)) // embedded at build time; a test catches it
	}
	var out []string
	for _, d := range t.Templates() {
		// ParseFS also names a template after each file, and the root is
		// "kernel"; neither is something a module could collide with by
		// defining it.
		if n := d.Name(); n != "kernel" && !strings.HasSuffix(n, ".html.tmpl") {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func parseKernel() (*template.Template, error) {
	return template.New("kernel").Funcs(funcs).ParseFS(kernelTemplates, "templates/*.html.tmpl")
}

func firstRecords(n int, recs []Record) []Record {
	if n < len(recs) {
		return recs[:n]
	}
	return recs
}

// funcs is the summary template's set (spec §6), over view records.
var funcs = template.FuncMap{
	"first": firstRecords,
	"date":  func(t time.Time) string { return t.UTC().Format("2006-01-02") },
	"age":   func(t time.Time, now time.Time) int { return int(now.Sub(t).Hours() / 24) },
}

// Nav is one entry of the navigation: Notes marks a working-memory module,
// listed apart as the model's notes (spec §10).
type Nav struct {
	Name, Href, Intro string
	Notes, Current    bool
}

// Faults are the deployment's, never the person's shortfall (spec §8.1, §10).
type Faults struct {
	Manual, Total     int
	LastRun, Interval string
	NoEvidence, Stale []Claim
	Block             []block.Fault
}

// Home is what the home page shows. Now dates the page; when it is zero the
// page carries no "as of" line.
type Home struct {
	Nav          []Nav
	Parts        []block.Part
	Instructions []instructions.Record
	Sizes        []instructions.Size
	Due          []agenda.Item
	Behind       []Claim
	Faults       Faults
	Now          time.Time
	// Links maps a record's path to its link (see Links), for the due
	// items and the agenda line.
	Links map[string]string
}

// ModulePage is one module's page: its kinds in their layouts, or, for a
// working-memory module, its notes.
type ModulePage struct {
	Nav    []Nav
	Module module.Manifest
	Kinds  []KindPage
	Notes  []Record // a working-memory module's records, listed by name and description
}

type Renderer struct {
	kernel  *template.Template
	modules map[string]*template.Template // "<module>/<kind>" -> a clone of kernel with the module's template
	md      goldmark.Markdown
}

// New parses the kernel's templates and every module view template. A module
// template that does not parse, or that defines a kernel name, is an error
// here, at startup, as a broken summary template is (spec §6).
func New(set *module.Set) (*Renderer, error) {
	// A fresh parse, not the one Reserved was read from: html/template will
	// not clone a template that has executed, and the clones are made here.
	kernel, err := parseKernel()
	if err != nil {
		return nil, fmt.Errorf("view: kernel templates: %w", err)
	}
	r := &Renderer{kernel: kernel, modules: map[string]*template.Template{}, md: goldmark.New()}
	reserved := map[string]bool{}
	for _, n := range Reserved {
		reserved[n] = true
	}
	for _, m := range set.Modules {
		for kname, k := range m.Kinds {
			if k.View == nil || k.View.Template == "" {
				continue
			}
			b, err := os.ReadFile(filepath.Join(m.Dir, k.View.Template))
			if err != nil {
				return nil, fmt.Errorf("module %s: kind %s: view template: %w", m.Name, kname, err)
			}
			probe, err := template.New("kind:" + kname).Funcs(funcs).Parse(string(b))
			if err != nil {
				return nil, fmt.Errorf("module %s: %s: %w", m.Name, k.View.Template, err)
			}
			// The probe's own name is "kind:<kind>", never reserved, so only
			// names the template itself defines are checked.
			for _, d := range probe.Templates() {
				if reserved[d.Name()] {
					return nil, fmt.Errorf("module %s: %s defines %q, which is the kernel's (spec §6)", m.Name, k.View.Template, d.Name())
				}
			}
			t, err := kernel.Clone()
			if err != nil {
				return nil, err
			}
			if _, err := t.New("kind:" + kname).Parse(string(b)); err != nil {
				return nil, fmt.Errorf("module %s: %s: %w", m.Name, k.View.Template, err)
			}
			r.modules[m.Name+"/"+kname] = t
		}
	}
	return r, nil
}

// Markdown renders s with raw HTML disabled, goldmark's default, so a record
// cannot put markup on the page (spec §10).
func (r *Renderer) Markdown(s string) template.HTML {
	var buf bytes.Buffer
	if err := r.md.Convert([]byte(s), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(s))
	}
	return template.HTML(buf.String()) // goldmark escapes text and omits raw HTML
}

// navItem is a Nav entry with its label as the page shows it.
type navItem struct {
	Nav
	Label string
}

// frame is what the shared page template receives: the record modules and
// the model's notes apart, so the notes group appears only when it has
// entries.
type frame struct {
	Title          string
	Home           bool
	Records, Notes []navItem
	Main           template.HTML
}

// page renders body into the shared frame: the head, the stylesheet link,
// the icon sprite and the navigation.
func (r *Renderer) page(w io.Writer, title string, home bool, nav []Nav, body string, data any) error {
	var main bytes.Buffer
	if err := r.kernel.ExecuteTemplate(&main, body, data); err != nil {
		return err
	}
	f := frame{Title: title, Home: home, Main: template.HTML(main.String())}
	for _, n := range nav {
		it := navItem{Nav: n, Label: capital(n.Name)}
		if n.Notes {
			f.Notes = append(f.Notes, it)
		} else {
			f.Records = append(f.Records, it)
		}
	}
	return r.kernel.ExecuteTemplate(w, "page", f)
}

// agendaLine is the block's first line, split as the page shows it: the
// record reference, linked to its page, and the question.
type agendaLine struct {
	Line      string // the whole line, used when it names no record
	Ref, Href string
	Question  string
}

func agendaOf(p block.Part, items []agenda.Item, links map[string]string) agendaLine {
	line := strings.TrimRight(p.Text, "\n")
	a := agendaLine{Line: line}
	rest, ok := strings.CutPrefix(line, "agenda: [")
	if !ok {
		return a
	}
	ref, q, ok := strings.Cut(rest, "] ")
	if !ok {
		return a
	}
	a.Ref, a.Question = "["+ref+"]", q
	if top, ok := agenda.Top(items); ok {
		a.Href = itemHref(top, links)
	}
	return a
}

// itemHref is where an agenda item points: its record's link from links,
// the instructions on the home page for a budget item, or the module's
// page for an onboarding item, which names no record. A record with no
// link has no page the caller can open, so it gets none.
func itemHref(it agenda.Item, links map[string]string) string {
	switch {
	case it.Reason == agenda.Budget:
		return "#instructions"
	case it.Path != "":
		return links[it.Path]
	case it.Module == "":
		return ""
	}
	return "/view/" + it.Module + "/"
}

// share is one module's part of the block: its first line as the key, and
// the rest as lines with their "- " marker removed.
type share struct {
	Module, Key string
	Lines       []string
}

func shareOf(p block.Part) share {
	lines := strings.Split(strings.TrimRight(p.Text, "\n"), "\n")
	s := share{Module: p.Module, Key: lines[0]}
	for _, l := range lines[1:] {
		s.Lines = append(s.Lines, strings.TrimPrefix(l, "- "))
	}
	return s
}

// gauge is one module's instructions against their budget.
type gauge struct {
	Module                string
	Value, Max, Percent   int
	BytesText, BudgetText string
	Over                  bool
	OverText              string
}

func gaugeOf(s instructions.Size) gauge {
	g := gauge{Module: s.Module, Value: min(s.Bytes, s.Budget), Max: s.Budget,
		BytesText: commas(s.Bytes), BudgetText: commas(s.Budget)}
	if s.Budget > 0 {
		g.Percent = s.Bytes * 100 / s.Budget
	}
	if s.Bytes > s.Budget {
		g.Over, g.OverText = true, commas(s.Bytes-s.Budget)
	}
	return g
}

type instruction struct {
	Kind string // "Register", from the record's path
	HTML template.HTML
}

type dueItem struct {
	agenda.Item
	Href, Label string
}

func dueOf(it agenda.Item, links map[string]string) dueItem {
	label := it.Path
	switch {
	case it.Module != "" && it.Kind == "" && it.Reason == agenda.Budget:
		label = it.Module + " instructions"
	case it.Module != "" && it.Kind == "":
		label = it.Module
	case it.Module != "":
		label = it.Module + "/" + it.Kind
		if it.ID != "" {
			label += " " + it.ID
		} else if it.Name != "" {
			label += " " + it.Name
		}
	}
	return dueItem{Item: it, Href: itemHref(it, links), Label: label}
}

// faultReadings are the faults with the figures their readings show.
type faultReadings struct {
	Faults
	ManualPercent int
}

// homeData is Home as the template needs it: the block split into lines,
// the instructions rendered, and every figure computed.
type homeData struct {
	Home
	AsOf         string
	Agenda       agendaLine
	Shares       []share
	BlockBytes   string
	Gauges       []gauge
	Instructions []instruction
	Due          []dueItem
	Behind       []Claim
	Faults       faultReadings
}

func (r *Renderer) Home(w io.Writer, h Home) error {
	d := homeData{Home: h}
	if !h.Now.IsZero() {
		d.AsOf = h.Now.Format("Monday, 2 January 2006")
	}
	n := 0
	for _, p := range h.Parts {
		n += len(p.Text)
		if p.Module == "" {
			d.Agenda = agendaOf(p, h.Due, h.Links)
		} else {
			d.Shares = append(d.Shares, shareOf(p))
		}
	}
	d.BlockBytes = commas(n)
	for _, s := range h.Sizes {
		d.Gauges = append(d.Gauges, gaugeOf(s))
	}
	for _, in := range h.Instructions {
		kind := ""
		if parts := strings.Split(in.Path, "/"); len(parts) > 1 {
			kind = capital(parts[1])
		}
		d.Instructions = append(d.Instructions, instruction{Kind: kind, HTML: r.Markdown(in.Text)})
	}
	for _, it := range h.Due {
		d.Due = append(d.Due, dueOf(it, h.Links))
	}
	d.Behind = annotate(h.Behind)
	d.Faults = faultReadings{Faults: h.Faults}
	d.Faults.NoEvidence, d.Faults.Stale = annotate(h.Faults.NoEvidence), annotate(h.Faults.Stale)
	if h.Faults.Total > 0 {
		d.Faults.ManualPercent = h.Faults.Manual * 100 / h.Faults.Total
	}
	return r.page(w, "Home", true, h.Nav, "home-body", d)
}

// section is one kind's part of a module page, rendered by its layout or its
// module's template, under the kernel's heading.
type section struct {
	KindPage
	HTML template.HTML
}

// noteGroup is one scope's notes on a working-memory page.
type noteGroup struct {
	Scope, ID string
	Notes     []Record
}

// groupNotes groups notes by scope: global first, then the rest by name.
func groupNotes(notes []Record) []noteGroup {
	by := map[string][]Record{}
	for _, n := range notes {
		sc := n.Scope
		if sc == "" {
			sc = "global"
		}
		by[sc] = append(by[sc], n)
	}
	scopes := make([]string, 0, len(by))
	for sc := range by {
		scopes = append(scopes, sc)
	}
	sort.Slice(scopes, func(i, j int) bool {
		if (scopes[i] == "global") != (scopes[j] == "global") {
			return scopes[i] == "global"
		}
		return scopes[i] < scopes[j]
	})
	out := make([]noteGroup, 0, len(scopes))
	for _, sc := range scopes {
		out = append(out, noteGroup{Scope: sc, ID: "scope-" + strings.ReplaceAll(sc, "/", "-"), Notes: by[sc]})
	}
	return out
}

// Module renders a module page. Each kind is rendered on its own first, so
// a module template that fails costs only its own section (spec §6).
func (r *Renderer) Module(w io.Writer, p ModulePage) error {
	sections := make([]section, 0, len(p.Kinds))
	for _, k := range p.Kinds {
		if k.Title == "" {
			k.Title = capital(k.Kind)
		}
		var buf bytes.Buffer
		var err error
		if t, ok := r.modules[k.Module+"/"+k.Kind]; ok && k.View.Template != "" {
			err = t.ExecuteTemplate(&buf, "kind:"+k.Kind, k)
		} else {
			err = r.kernel.ExecuteTemplate(&buf, "layout-"+k.View.Layout, k)
		}
		if err != nil {
			log.Printf("view %s/%s: %v", k.Module, k.Kind, err)
			buf.Reset()
			if ferr := r.kernel.ExecuteTemplate(&buf, "render-failure", k); ferr != nil {
				return ferr
			}
		}
		sections = append(sections, section{KindPage: k, HTML: template.HTML(buf.String())})
	}
	return r.page(w, capital(p.Module.Name), false, p.Nav, "module-body", struct {
		ModulePage
		Title      string
		IsNotes    bool
		Sections   []section
		NoteGroups []noteGroup
		NoteCount  int
	}{p, capital(p.Module.Name), p.Module.Profile == module.WorkingMemory, sections, groupNotes(p.Notes), len(p.Notes)})
}
