package core

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// Library is the loaded, in-memory knowledge base.
type Library struct {
	Techniques []*Technique
	byID       map[string]*Technique
	loadErrs   []error
}

// Load walks fsys under root and parses every Markdown technique it finds.
//
// A broken file never aborts the load: its error is collected and reported so
// one bad contribution cannot take the whole guide down. Files and directories
// starting with "." or "_" are skipped, which gives contributors a way to park
// drafts (e.g. content/offensive/_wip/).
func Load(fsys fs.FS, root string) (*Library, error) {
	lib := &Library{byID: make(map[string]*Technique)}
	cleanRoot := path.Clean(root)

	err := fs.WalkDir(fsys, cleanRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()

		if d.IsDir() {
			if p != cleanRoot && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			return nil
		}
		if !strings.EqualFold(path.Ext(name), ".md") {
			return nil
		}

		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			lib.loadErrs = append(lib.loadErrs, fmt.Errorf("%s: %w", p, err))
			return nil
		}

		rel := p
		if cleanRoot != "." {
			rel = strings.TrimPrefix(p, cleanRoot+"/")
		}
		t, err := Parse(rel, data)
		if err != nil {
			lib.loadErrs = append(lib.loadErrs, fmt.Errorf("%s: %w", p, err))
			return nil
		}

		if prev, dup := lib.byID[strings.ToLower(t.ID)]; dup {
			lib.loadErrs = append(lib.loadErrs, fmt.Errorf("duplicate id %q: %s and %s", t.ID, prev.Path, t.Path))
			return nil
		}
		lib.byID[strings.ToLower(t.ID)] = t
		lib.Techniques = append(lib.Techniques, t)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(lib.Techniques, func(i, j int) bool {
		a, b := lib.Techniques[i], lib.Techniques[j]
		if ac, bc := a.CategoryPath(), b.CategoryPath(); ac != bc {
			return ac < bc
		}
		return a.ID < b.ID
	})

	return lib, nil
}

// Errors returns the non-fatal problems found while loading (unreadable or
// malformed files, duplicate ids). The library is still usable.
func (l *Library) Errors() []error { return l.loadErrs }

// Len returns the number of loaded techniques.
func (l *Library) Len() int { return len(l.Techniques) }

// Get looks a technique up by id.
func (l *Library) Get(id string) (*Technique, bool) {
	t, ok := l.byID[strings.ToLower(strings.TrimSpace(id))]
	return t, ok
}

// Categories returns every category prefix seen in the library, sorted, with
// parents before children (so a UI can render a tree in one pass).
func (l *Library) Categories() []string {
	seen := make(map[string]bool)
	var out []string
	for _, t := range l.Techniques {
		for i := 1; i <= len(t.Category); i++ {
			c := strings.Join(t.Category[:i], "/")
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	sort.Strings(out)
	return out
}

// CountInCategory returns how many techniques live under a category prefix.
func (l *Library) CountInCategory(cat string) int {
	n := 0
	for _, t := range l.Techniques {
		if cat == "" {
			n++
			continue
		}
		if strings.HasPrefix(t.CategoryPath(), cat) {
			n++
		}
	}
	return n
}
