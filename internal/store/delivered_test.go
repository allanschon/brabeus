package store

import (
	"testing"

	"github.com/allanschon/brabeus/internal/module"
)

func TestDelivered(t *testing.T) {
	kind := module.Kind{Fields: []string{"statement"}}
	for _, c := range []struct {
		name string
		rec  Record
		kind module.Kind
		want string
	}{
		{"body wins", Record{Body: "Body.", Fields: map[string]string{"statement": "Field."}}, kind, "Body."},
		{"empty body falls back to the first field", Record{Fields: map[string]string{"statement": "Field."}}, kind, "Field."},
		{"whitespace-only body falls back", Record{Body: " \n", Fields: map[string]string{"statement": "Field."}}, kind, "Field."},
		{"both empty", Record{}, kind, ""},
		{"no declared field", Record{Fields: map[string]string{"statement": "Field."}}, module.Kind{}, ""},
		{"trimmed", Record{Body: "\n  Body.  \n"}, kind, "Body."},
		{"field trimmed", Record{Fields: map[string]string{"statement": "  Field. "}}, kind, "Field."},
	} {
		if got := Delivered(c.rec, c.kind); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
