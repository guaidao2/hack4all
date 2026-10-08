package core

import "strings"

// Query is a parsed search expression.
//
// Free words are ANDed, exactly as before. Field prefixes narrow the search
// first, which is what makes the non-interactive mode useful for an agent:
// it can ask a precise question instead of hoping free text ranks well.
//
//	kerberos                    free text
//	category:offensive/web      only the category subtree
//	tag:active-directory        tag must be present
//	tool:hashcat                tool must be present
//	attck:T1558.003             ATT&CK id must be present
//	platform:windows            platform must be present
//	difficulty:intermediate     difficulty must be present
//	id:kerberoasting            exact technique id
//
// Field values match as case-insensitive substrings, so `attck:T1558` finds
// T1558.003 and `tool:hash` finds hashcat. An unrecognised prefix such as
// `foo:bar` stays a free-text word: a query must never be silently swallowed
// by a field the tool does not know about.
type Query struct {
	Raw        string
	Terms      []string
	ID         string
	Category   string
	Tags       []string
	Tools      []string
	ATTACK     []string
	Platform   []string
	Difficulty string
}

// ParseQuery splits a raw search string into field constraints and free terms.
func ParseQuery(raw string) Query {
	q := Query{Raw: raw}

	for _, tok := range strings.Fields(raw) {
		key, val, ok := strings.Cut(tok, ":")
		if !ok || val == "" {
			q.Terms = append(q.Terms, strings.ToLower(tok))
			continue
		}

		switch strings.ToLower(key) {
		case "id":
			q.ID = strings.ToLower(val)
		case "category", "cat":
			q.Category = strings.Trim(strings.ToLower(val), "/")
		case "tag", "tags":
			q.Tags = append(q.Tags, strings.ToLower(val))
		case "tool", "tools":
			q.Tools = append(q.Tools, strings.ToLower(val))
		case "attck", "attack", "mitre":
			q.ATTACK = append(q.ATTACK, strings.ToLower(val))
		case "platform", "os":
			q.Platform = append(q.Platform, strings.ToLower(val))
		case "difficulty", "diff", "level":
			q.Difficulty = strings.ToLower(val)
		default:
			q.Terms = append(q.Terms, strings.ToLower(tok))
		}
	}

	return q
}

// IsEmpty reports whether the query constrains nothing at all.
func (q Query) IsEmpty() bool {
	return len(q.Terms) == 0 &&
		q.ID == "" &&
		q.Category == "" &&
		len(q.Tags) == 0 &&
		len(q.Tools) == 0 &&
		len(q.ATTACK) == 0 &&
		len(q.Platform) == 0 &&
		q.Difficulty == ""
}

// Match scores a technique against the query. ok is false when the technique
// fails any part of it.
func (q Query) Match(t *Technique) (score int, fields []string, ok bool) {
	if q.ID != "" {
		if !strings.EqualFold(t.ID, q.ID) {
			return 0, nil, false
		}
		score += 1000
		fields = append(fields, "id")
	}

	if q.Category != "" {
		p := strings.ToLower(t.CategoryPath())
		if p != q.Category && !strings.HasPrefix(p, q.Category+"/") {
			return 0, nil, false
		}
		score += 20
		fields = append(fields, "category")
	}

	if !everyNeedleIn(q.Tags, t.Tags) {
		return 0, nil, false
	}
	if len(q.Tags) > 0 {
		score += 8 * len(q.Tags)
		fields = append(fields, "tags")
	}

	if !everyNeedleIn(q.Tools, t.Tools) {
		return 0, nil, false
	}
	if len(q.Tools) > 0 {
		score += 8 * len(q.Tools)
		fields = append(fields, "tools")
	}

	if !everyNeedleIn(q.ATTACK, t.ATTACK) {
		return 0, nil, false
	}
	if len(q.ATTACK) > 0 {
		score += 8 * len(q.ATTACK)
		fields = append(fields, "attck")
	}

	if !everyNeedleIn(q.Platform, t.Platform) {
		return 0, nil, false
	}
	if len(q.Platform) > 0 {
		score += 4 * len(q.Platform)
		fields = append(fields, "platform")
	}

	if q.Difficulty != "" {
		if !strings.Contains(strings.ToLower(t.Difficulty), q.Difficulty) {
			return 0, nil, false
		}
		score += 4
		fields = append(fields, "difficulty")
	}

	if len(q.Terms) > 0 {
		termScore, termFields, ok := scoreTechnique(t, q.Terms)
		if !ok {
			return 0, nil, false
		}
		score += termScore
		fields = append(fields, termFields...)
	}

	return score, fields, true
}

// everyNeedleIn reports whether every needle appears, case-insensitively, as a
// substring of at least one haystack entry.
func everyNeedleIn(needles, haystack []string) bool {
	if len(needles) == 0 {
		return true
	}

	lowered := make([]string, len(haystack))
	for i, h := range haystack {
		lowered[i] = strings.ToLower(h)
	}

	for _, n := range needles {
		found := false
		for _, h := range lowered {
			if strings.Contains(h, n) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
