package core

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// Issue is one problem found while validating the knowledge base.
type Issue struct {
	Level string // "error" or "warning"
	Path  string // the file it belongs to; empty for library-wide problems
	Msg   string
}

var kebabRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Validate reports everything broken or questionable about the loaded library.
//
// An error means content is broken or has been lost — a file that will not
// parse, a duplicate id. A warning means the entry works but breaks a house
// rule, and those rules exist for the reader: English is the primary language,
// Chinese is expected next to it, and a half-translated entry should say so out
// loud rather than quietly showing one language forever.
func (l *Library) Validate() []Issue {
	issues := make([]Issue, 0, len(l.loadErrs))

	for _, err := range l.loadErrs {
		issues = append(issues, Issue{Level: "error", Msg: err.Error()})
	}
	for _, t := range l.Techniques {
		issues = append(issues, validateTechnique(t)...)
	}

	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Path != issues[j].Path {
			return issues[i].Path < issues[j].Path
		}
		if issues[i].Level != issues[j].Level {
			return issues[i].Level < issues[j].Level
		}
		return issues[i].Msg < issues[j].Msg
	})

	return issues
}

func validateTechnique(t *Technique) []Issue {
	var issues []Issue
	add := func(format string, args ...any) {
		issues = append(issues, Issue{
			Level: "warning",
			Path:  t.Path,
			Msg:   fmt.Sprintf(format, args...),
		})
	}

	base := strings.TrimSuffix(path.Base(t.Path), path.Ext(t.Path))
	if !kebabRe.MatchString(base) {
		add("file name is not kebab-case: %q", base)
	}
	if !strings.EqualFold(base, t.ID) {
		add("id %q does not match the file name %q", t.ID, base)
	}
	if t.Title.EN == "" {
		add("missing title_en (English is the primary language)")
	}
	if t.Title.ZH == "" {
		add("missing title_zh")
	}
	if t.Summary.EN == "" || t.Summary.ZH == "" {
		add("missing summary_en or summary_zh")
	}
	if t.Body.EN == "" {
		add("no English body section (<!-- lang:en -->)")
	}
	if t.Body.ZH == "" {
		add("no Chinese body section (<!-- lang:zh -->)")
	}
	if len(t.Tags) == 0 {
		add("no tags")
	}
	if t.Updated == "" {
		add("missing updated date")
	}
	if len(t.Category) == 0 {
		add("sits at the content root; give it a category directory")
	}
	for _, seg := range t.Category {
		if !kebabRe.MatchString(seg) {
			add("category directory is not kebab-case: %q", seg)
		}
	}

	return issues
}

// CountIssues splits a report by level.
func CountIssues(issues []Issue) (errors, warnings int) {
	for _, is := range issues {
		if is.Level == "error" {
			errors++
		} else {
			warnings++
		}
	}
	return errors, warnings
}
