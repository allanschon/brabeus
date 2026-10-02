package instructions_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/allanschon/brabeus/internal/instructions"
	"github.com/allanschon/brabeus/internal/module"
	"github.com/allanschon/brabeus/internal/store"
)

func loadModules(t *testing.T, names ...string) *module.Set {
	t.Helper()
	set, err := module.Load(filepath.Join("..", "..", "modules"), names)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func TestCollectDeliversOnlyConfirmedInstructions(t *testing.T) {
	set := loadModules(t, "memory", "identity", "telos")
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	rec := func(path, mod, kind, body string, reviewed, retired bool) store.Stored {
		s := store.Stored{Path: path}
		s.Module, s.Kind, s.Body = mod, kind, body
		s.Fields = map[string]string{"statement": "stmt of " + path}
		if reviewed {
			s.Reviewed = now
		}
		if retired {
			s.Retired = now
		}
		return s
	}
	recs := []store.Stored{
		rec("identity/preference/b.md", "identity", "preference", "Rule B.", true, false),
		rec("identity/preference/a.md", "identity", "preference", "Rule A.", true, false),
		rec("identity/preference/draft.md", "identity", "preference", "Never delivered.", false, false),
		rec("identity/preference/gone.md", "identity", "preference", "Retired.", true, true),
		rec("identity/register/r.md", "identity", "register", "", true, false),
		rec("identity/value/v.md", "identity", "value", "A value, not an instruction.", true, false),
		rec("memory/preference/x.md", "memory", "preference", "Crossed and confirmed.", true, false),
		rec("memory/preference/y.md", "memory", "preference", "Crossed, not confirmed.", false, false),
	}
	got := instructions.Collect(set, recs)
	var paths []string
	for _, r := range got {
		paths = append(paths, r.Path)
	}
	want := []string{"identity/preference/a.md", "identity/preference/b.md", "identity/register/r.md", "memory/preference/x.md"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	if got[2].Text != "stmt of identity/register/r.md" {
		t.Errorf("empty body should fall back to the first field, got %q", got[2].Text)
	}
	if got[3].Module != "identity" {
		t.Errorf("a crossing preference is governed by identity, got %q", got[3].Module)
	}
}

func TestWithoutIdentityNothingIsAnInstruction(t *testing.T) {
	set := loadModules(t, "memory")
	s := store.Stored{Path: "memory/preference/x.md"}
	s.Module, s.Kind, s.Body = "memory", "preference", "Rule."
	s.Reviewed = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	got := instructions.Collect(set, []store.Stored{s})
	if len(got) != 0 {
		t.Errorf("Collect = %v, want none", got)
	}
	if sizes := instructions.Sizes(set, got); len(sizes) != 0 {
		t.Errorf("Sizes = %v, want none", sizes)
	}
}

func TestSizesMeasureTheJoinedText(t *testing.T) {
	set := loadModules(t, "memory", "identity")
	recs := []instructions.Record{
		{Module: "identity", Path: "identity/preference/a.md", Text: "abc"},
		{Module: "identity", Path: "identity/preference/b.md", Text: "de"},
	}
	want := []instructions.Size{{Module: "identity", Bytes: 7, Budget: 4096}}
	if got := instructions.Sizes(set, recs); !reflect.DeepEqual(got, want) {
		t.Errorf("Sizes = %v, want %v", got, want)
	}
	want = []instructions.Size{{Module: "identity", Bytes: 0, Budget: 4096}}
	if got := instructions.Sizes(set, nil); !reflect.DeepEqual(got, want) {
		t.Errorf("Sizes with none = %v, want %v", got, want)
	}
}

func TestTextIsEmptyForNoRecords(t *testing.T) {
	if got := instructions.Text(nil); got != "" {
		t.Errorf("Text(nil) = %q, want empty", got)
	}
}

func TestTextOpensWithTheOpeningLine(t *testing.T) {
	got := instructions.Text([]instructions.Record{{Text: "One."}, {Text: "Two."}})
	want := instructions.Opening + "\n\nOne.\n\nTwo."
	if got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
	if !strings.HasPrefix(got, "These are the person's standing instructions") {
		t.Errorf("Text does not open with the opening line: %q", got)
	}
}
