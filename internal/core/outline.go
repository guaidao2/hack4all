package core

import (
	"regexp"
	"strings"
)

// Heading is one section title inside a technique body.
type Heading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
}

var headingLineRe = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

// Headings extracts the Markdown section titles of a technique.
//
// Lines inside fenced code blocks are skipped on purpose: these files are full
// of shell comments starting with "#", and reporting those as sections would
// make the outline worse than useless.
func (t *Technique) Headings(lang string) []Heading {
	body, _ := t.Body.Get(lang)

	var out []Heading
	inFence := false

	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		m := headingLineRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		out = append(out, Heading{Level: len(m[1]), Text: strings.TrimSpace(m[2])})
	}

	return out
}
