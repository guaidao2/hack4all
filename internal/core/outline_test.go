package core

import "testing"

const outlineFile = `---
id: outline-sample
title_en: Outline Sample
---

<!-- lang:en -->
## First section

Some text with a shell block:

` + "```bash" + `
# this is a comment, not a heading
echo hi
` + "```" + `

### Subsection

More text.
`

func TestHeadingsSkipCodeBlocks(t *testing.T) {
	tech := mustParse(t, "web/outline-sample.md", outlineFile)
	hs := tech.Headings(LangEN)

	if len(hs) != 2 {
		t.Fatalf("got %d headings, want 2 — the shell comment must not count as one: %+v", len(hs), hs)
	}
	if hs[0].Level != 2 || hs[0].Text != "First section" {
		t.Errorf("first heading = %+v", hs[0])
	}
	if hs[1].Level != 3 || hs[1].Text != "Subsection" {
		t.Errorf("second heading = %+v", hs[1])
	}
}

func TestHeadingsFollowTheRequestedLanguage(t *testing.T) {
	// sampleFile (see core_test.go) has a Chinese section called 原理.
	tech := mustParse(t, "web/sample-technique.md", sampleFile)

	hs := tech.Headings(LangZH)
	if len(hs) == 0 || hs[0].Text != "原理" {
		t.Errorf("zh headings = %+v", hs)
	}
	if en := tech.Headings(LangEN); len(en) == 0 || en[0].Text != "Why it works" {
		t.Errorf("en headings = %+v", en)
	}
}

func TestViewCarriesHeadingsOnlyWhenAsked(t *testing.T) {
	tech := mustParse(t, "web/outline-sample.md", outlineFile)

	if v := tech.View(LangEN, 0); len(v.Headings) != 0 {
		t.Errorf("a plain View should not carry headings, got %d", len(v.Headings))
	}
	if v := tech.ViewWithHeadings(LangEN, 0); len(v.Headings) != 2 {
		t.Errorf("ViewWithHeadings returned %d headings, want 2", len(v.Headings))
	}
}
