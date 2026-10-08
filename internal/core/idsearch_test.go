package core

import (
	"strings"
	"testing"
)

// The id is derived from the filename and is what people see in the TUI, so a
// fragment of it has to match without the user knowing about the id: prefix.
// This is also what makes related entries discoverable by prefix: every classical
// cipher entry starts with classical-ciphers-, so the prefix finds the series.
func TestSearchFindsEntriesByIDFragment(t *testing.T) {
	lib := loadEmbeddedLibrary(t)

	for _, q := range []string{"classical-ciphers", "red-team-tactical", "lab-setup"} {
		matches := lib.Search(q, 0)
		if len(matches) == 0 {
			t.Errorf("no match for %q", q)
			continue
		}

		found := false
		for _, m := range matches {
			if strings.Contains(m.Technique.ID, q) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%q matched %d entries but none of them has it in the id", q, len(matches))
		}
	}
}

// A series prefix should pull out every entry in the series.
func TestSearchBySeriesPrefix(t *testing.T) {
	lib := loadEmbeddedLibrary(t)

	var want int
	for _, tech := range lib.Techniques {
		if strings.HasPrefix(tech.ID, "classical-ciphers-") {
			want++
		}
	}
	if want < 2 {
		t.Skipf("only %d classical cipher entries exist; nothing to assert yet", want)
	}

	if got := len(lib.Search("classical-ciphers", 0)); got < want {
		t.Errorf("prefix search returned %d entries, but %d share the prefix", got, want)
	}
	// The id: prefix still means an exact match, and must not have been loosened.
	if got := len(lib.Search("id:classical-ciphers", 0)); got != 0 {
		t.Errorf("id:classical-ciphers matched %d entries, want 0 (it is a prefix, not an id)", got)
	}
}
