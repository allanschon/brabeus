package view

import (
	"html/template"
	"path/filepath"
	"testing"
	"time"

	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/store"
)

func shipped(t *testing.T) *module.Set {
	t.Helper()
	set, err := module.Load(filepath.Join("..", "..", "modules"), []string{"memory", "identity", "telos"})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func plain(s string) template.HTML { return template.HTML(template.HTMLEscapeString(s)) }

var now = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

func stored(path, mod, kind string, reviewed time.Time) store.Stored {
	s := store.Stored{Path: path}
	s.Module, s.Kind, s.Name, s.Scope = mod, kind, filepath.Base(path[:len(path)-3]), "global"
	s.Fields = map[string]string{"statement": "x"}
	s.Reviewed = reviewed
	return s
}

// Spec §7, §10: a crossing preference is the model's note until reviewed,
// and the person's identity record after; it is listed on exactly one page.
func TestACrossingPreferenceIsListedOnOnePage(t *testing.T) {
	set := shipped(t)
	draft := stored("memory/preference/terse.md", "memory", "preference", time.Time{})
	confirmed := stored("memory/preference/plain.md", "memory", "preference", now.Add(-time.Hour))
	if got := PageModule(set, draft); got != "memory" {
		t.Errorf("unreviewed crossing preference on %q, want memory", got)
	}
	if got := PageModule(set, confirmed); got != "identity" {
		t.Errorf("reviewed crossing preference on %q, want identity", got)
	}
	if got := PageModule(set, stored("telos/goal/g.md", "telos", "goal", now)); got != "telos" {
		t.Errorf("own record on %q", got)
	}
}

// Spec §10: freshness is time since reviewed against the kind's
// freshness_days, or unconfirmed; snoozes and claims ride with the record.
func TestBuildRecordsAnnotatesFreshnessClaimsAndServes(t *testing.T) {
	set := shipped(t)
	goal := stored("telos/goal/g1.md", "telos", "goal", now.AddDate(0, 0, -100))
	goal.ID, goal.Snoozes = "G1", 2
	goal.Fields = map[string]string{"id": "G1", "title": "Cull photos", "by": "2026-12-31", "serves": "sharing, missing-value"}
	value := stored("identity/value/sharing.md", "identity", "value", time.Time{})
	claims := []Claim{{Goal: "telos/goal/g1.md", Index: 0, Text: "all reviewed", State: "behind"}}
	got := BuildRecords(set, []store.Stored{goal, value}, claims, plain, now)
	var g, v Record
	for _, r := range got {
		switch r.Path {
		case goal.Path:
			g = r
		case value.Path:
			v = r
		}
	}
	if g.Freshness.Age != 100 || g.Freshness.Every != 90 || !g.Freshness.Overdue || g.Freshness.Unconfirmed {
		t.Errorf("goal freshness = %+v", g.Freshness)
	}
	if !v.Freshness.Unconfirmed {
		t.Errorf("a never-reviewed record is unconfirmed: %+v", v.Freshness)
	}
	if g.Snoozes != 2 || len(g.Claims) != 1 || g.Claims[0].State != "behind" {
		t.Errorf("snoozes/claims = %d %+v", g.Snoozes, g.Claims)
	}
	if len(g.Serves) != 2 || g.Serves[0].Href != "/view/identity/#sharing" || g.Serves[1].Href != "" {
		t.Errorf("serves = %+v; a name with no record keeps no link", g.Serves)
	}
}

func TestKindsGroupsByKindAndOmitsEmptyKinds(t *testing.T) {
	set := shipped(t)
	recs := BuildRecords(set, []store.Stored{
		stored("telos/mission/m.md", "telos", "mission", now),
		stored("telos/problem/p.md", "telos", "problem", now),
	}, nil, plain, now)
	pages := Kinds(set, "telos", recs, now)
	if len(pages) != 2 || pages[0].Kind != "mission" || pages[1].Kind != "problem" {
		t.Errorf("kinds = %+v", pages)
	}
	if pages[0].View.Layout != "list" {
		t.Errorf("mission has no view key, so it is a list: %+v", pages[0].View)
	}
}
