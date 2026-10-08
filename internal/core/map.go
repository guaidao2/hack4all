package core

import (
	"regexp"
	"strconv"
	"strings"
)

// DefaultMapID is the coverage map the web UI draws when no other is requested.
const DefaultMapID = "red-team-tactical-map"

// TacticalMap is the structured form of a checklist entry.
//
// The coverage map is written as Markdown so it can be read in a terminal and
// searched like everything else, and parsed here so it can also be drawn. One
// file, two presentations: the text version and the picture can never drift
// apart, which is the same rule the three front-ends follow.
type TacticalMap struct {
	Title    string       `json:"title"`
	Stages   []MapStage   `json:"stages"`
	Surfaces []MapSurface `json:"surfaces,omitempty"`
	TopMiss  []string     `json:"topMissed,omitempty"`
}

// MapStage is one phase of an engagement: the questions that decide whether it
// was covered, plus what that phase usually loses.
type MapStage struct {
	Number  int      `json:"number"`
	Title   string   `json:"title"`
	Items   []string `json:"items"`
	Missed  string   `json:"missed,omitempty"`
	Related []string `json:"related,omitempty"`
}

// MapSurface is one of the axes that cut across every stage.
type MapSurface struct {
	Name    string   `json:"name"`
	Why     string   `json:"why,omitempty"`
	Related []string `json:"related,omitempty"`
}

var (
	stageRe   = regexp.MustCompile(`^####\s+(\d+)\.\s+(.+?)\s*$`)
	itemRe    = regexp.MustCompile(`^-\s+\[\s*\]\s+(.+?)\s*$`)
	missedRe  = regexp.MustCompile(`^\*\*(?:Often missed|常被漏掉)[^*]*\*\*\s*(.+?)\s*$`)
	relatedRe = regexp.MustCompile(`^(?:Related|相关)\s*[:：]\s*(.+?)\s*$`)
	tableRe   = regexp.MustCompile(`^\|.*\|\s*$`)
	entryIDRe = regexp.MustCompile(`id:([a-z0-9][a-z0-9-]*)`)
	topMissRe = regexp.MustCompile(`^\d+\.\s+\*\*(.+?)\*\*(.*?)\s*$`)
)

// TacticalMapOf parses a technique body into a coverage map.
//
// It returns nil when the entry is not written in that form, which is true of
// every other technique in the library — so the parser needs no flag in the
// frontmatter to tell the two kinds of content apart.
func TacticalMapOf(t *Technique, lang string) *TacticalMap {
	if t == nil {
		return nil
	}
	body, _ := t.Body.Get(lang)
	if strings.TrimSpace(body) == "" {
		return nil
	}

	m := &TacticalMap{Title: t.TitleFor(lang)}
	lines := strings.Split(body, "\n")
	var cur *MapStage

	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], " \t")

		if mm := stageRe.FindStringSubmatch(line); mm != nil {
			n, _ := strconv.Atoi(mm[1])
			m.Stages = append(m.Stages, MapStage{Number: n, Title: mm[2]})
			cur = &m.Stages[len(m.Stages)-1]
			continue
		}

		if mm := itemRe.FindStringSubmatch(line); mm != nil {
			if cur != nil {
				cur.Items = append(cur.Items, mm[1])
			}
			continue
		}

		if mm := missedRe.FindStringSubmatch(line); mm != nil {
			if cur != nil {
				cur.Missed = mm[1]
			}
			continue
		}

		if mm := relatedRe.FindStringSubmatch(line); mm != nil {
			ids := extractEntryIDs(mm[1])
			switch {
			case len(ids) == 0:
			case cur != nil:
				cur.Related = append(cur.Related, ids...)
			case len(m.Surfaces) > 0:
				last := &m.Surfaces[len(m.Surfaces)-1]
				last.Related = append(last.Related, ids...)
			}
			continue
		}

		if tableRe.MatchString(line) {
			cells := splitTableRow(line)
			// Skip the rule row and the header above it: the header is the row
			// whose successor is the rule.
			if len(cells) < 2 || isTableRule(cells) {
				continue
			}
			// The header is the row whose successor is a rule row — and the
			// successor has to be a table row itself, because a blank line also
			// trims down to something isTableRule would call a rule.
			if i+1 < len(lines) {
				next := strings.TrimRight(lines[i+1], " \t")
				if tableRe.MatchString(next) && isTableRule(splitTableRow(next)) {
					continue
				}
			}
			m.Surfaces = append(m.Surfaces, MapSurface{
				Name:    cells[0],
				Why:     cells[1],
				Related: extractEntryIDs(cells[len(cells)-1]),
			})
			continue
		}

		// Numbered bold lines are the closing "most missed" list. Stage titles are
		// headings and checklist items are dashes, so this shape is unambiguous.
		if mm := topMissRe.FindStringSubmatch(line); mm != nil {
			if text := strings.TrimSpace(mm[1] + mm[2]); text != "" {
				m.TopMiss = append(m.TopMiss, text)
			}
		}
	}

	if len(m.Stages) == 0 {
		return nil
	}
	return m
}

// extractEntryIDs pulls the technique ids out of a "Related: ..." line, where
// they are written as the query a reader could run: `-x id:kerberoasting`.
func extractEntryIDs(s string) []string {
	var out []string
	for _, m := range entryIDRe.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}

func splitTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

// isTableRule reports whether a row is the dashes under a Markdown table header.
func isTableRule(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		if strings.Trim(c, "-: ") != "" {
			return false
		}
	}
	return true
}
