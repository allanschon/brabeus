package store

import "time"

// at is the store tests' one stamp parser (K1): a fixture with a stamp that
// does not parse is a broken test, so it fails loudly rather than yield zero.
func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
