package core

import (
	"sort"
	"strings"
)

// Match is a single search hit.
type Match struct {
	Technique *Technique
	Score     int
	Fields    []string // which parts of the entry matched: title, meta, summary, body
}

// fieldWeights decides the ranking. A hit in the title matters far more than a
// hit somewhere in the middle of a long body, otherwise a technique that merely
// mentions a word outranks the technique that is about it.
var fieldWeights = []struct {
	key    string
	weight int
}{
	{"title", 12},
	{"meta", 8}, // tags, tools, ATT&CK ids, platform, category, difficulty
	{"summary", 4},
	{"body", 2},
}

// Search runs a case-insensitive search over the library.
//
// Terms are ANDed: every word in the query must appear somewhere in the entry.
// Both languages are indexed at once, so a Chinese query finds an entry whose
// English text is what actually contains the term, and vice versa. Chinese has
// no word boundaries, so substring matching does the right thing there; for
// English the AND-of-terms rule keeps results sane.
//
// An exact id match always wins and is returned first. A limit <= 0 means "no
// limit".
func (l *Library) Search(query string, limit int) []Match {
	q := strings.ToLower(strings.TrimSpace(query))

	if q == "" {
		all := make([]Match, 0, len(l.Techniques))
		for _, t := range l.Techniques {
			all = append(all, Match{Technique: t})
		}
		return clampLimit(all, limit)
	}

	var out []Match

	// Exact id (or exact title) beats everything else.
	if t, ok := l.Get(q); ok {
		out = append(out, Match{Technique: t, Score: 1000, Fields: []string{"id"}})
	}

	terms := strings.Fields(q)
	for _, t := range l.Techniques {
		if len(out) > 0 && out[0].Technique == t {
			continue
		}
		score, fields, ok := scoreTechnique(t, terms)
		if !ok {
			continue
		}
		out = append(out, Match{Technique: t, Score: score, Fields: fields})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Technique.ID < out[j].Technique.ID
	})

	return clampLimit(out, limit)
}

// InCategory filters the library by a category prefix ("offensive" or
// "offensive/credential-access"). An empty prefix returns everything.
func (l *Library) InCategory(cat string) []*Technique {
	cat = strings.Trim(strings.TrimSpace(cat), "/")
	if cat == "" {
		return l.Techniques
	}
	var out []*Technique
	for _, t := range l.Techniques {
		if t.CategoryPath() == cat || strings.HasPrefix(t.CategoryPath(), cat+"/") {
			out = append(out, t)
		}
	}
	return out
}

func scoreTechnique(t *Technique, terms []string) (int, []string, bool) {
	total := 0
	var fields []string

	for _, term := range terms {
		termScore := 0
		for _, fw := range fieldWeights {
			if strings.Contains(t.index[fw.key], term) {
				termScore += fw.weight
				if !contains(fields, fw.key) {
					fields = append(fields, fw.key)
				}
			}
		}
		// AND semantics: one unmatched term drops the entry entirely.
		if termScore == 0 {
			return 0, nil, false
		}
		total += termScore
	}

	return total, fields, true
}

func clampLimit(in []Match, limit int) []Match {
	if limit > 0 && len(in) > limit {
		return in[:limit]
	}
	return in
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
