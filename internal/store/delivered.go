package store

import (
	"strings"

	"github.com/allanschon/brabeus/internal/module"
)

// Delivered is the text of one instruction record as a session receives it:
// the record's body, trimmed; when the body is empty, the value of the kind's
// first declared field, for a record written with only its field.
func Delivered(r Record, k module.Kind) string {
	if text := strings.TrimSpace(r.Body); text != "" {
		return text
	}
	if len(k.Fields) == 0 {
		return ""
	}
	return strings.TrimSpace(r.Fields[k.Fields[0]])
}
