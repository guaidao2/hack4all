package core

import (
	"strings"
	"testing"
	"testing/fstest"
)

// A canonical two-language entry, used by most tests below.
const sampleFile = `---
id: sample-technique
title_en: Sample Technique
title_zh: 示例技术
summary_en: English summary line.
summary_zh: 中文摘要行。
tags: [web, test]
tools: [curl]
attck: [T1234.001]
platform: [linux]
difficulty: beginner
updated: 2026-01-01
---

<!-- lang:en -->
### Why it works

English body about kerberos and relays.

<!-- lang:zh -->
### 原理

中文正文，讲 kerberos 与中继。
`

func mustParse(t *testing.T, relPath, data string) *Technique {
	t.Helper()
	tech, err := Parse(relPath, []byte(data))
	if err != nil {
		t.Fatalf("Parse(%s) returned error: %v", relPath, err)
	}
	return tech
}

// ------------------------------------------------------------------ parsing

func TestParseFrontmatter(t *testing.T) {
	tech := mustParse(t, "offensive/web/sample-technique.md", sampleFile)

	if tech.ID != "sample-technique" {
		t.Errorf("ID = %q, want sample-technique", tech.ID)
	}
	if tech.Title.EN != "Sample Technique" || tech.Title.ZH != "示例技术" {
		t.Errorf("Title = %+v", tech.Title)
	}
	if len(tech.Tags) != 2 || tech.Tags[0] != "web" {
		t.Errorf("Tags = %v", tech.Tags)
	}
	if len(tech.ATTACK) != 1 || tech.ATTACK[0] != "T1234.001" {
		t.Errorf("ATTACK = %v", tech.ATTACK)
	}
	if tech.Difficulty != "beginner" || tech.Updated != "2026-01-01" {
		t.Errorf("Difficulty/Updated = %q/%q", tech.Difficulty, tech.Updated)
	}
}

func TestParseSplitsLanguages(t *testing.T) {
	tech := mustParse(t, "web/sample-technique.md", sampleFile)

	if !strings.Contains(tech.Body.EN, "English body") {
		t.Errorf("English body not captured: %q", tech.Body.EN)
	}
	if strings.Contains(tech.Body.EN, "中文正文") {
		t.Error("English body leaked Chinese content")
	}
	if !strings.Contains(tech.Body.ZH, "中文正文") {
		t.Errorf("Chinese body not captured: %q", tech.Body.ZH)
	}
	if strings.Contains(tech.Body.ZH, "English body") {
		t.Error("Chinese body leaked English content")
	}
}

func TestParseWithoutFrontmatter(t *testing.T) {
	// A file with no frontmatter is still valid; the whole thing is English.
	tech := mustParse(t, "web/plain.md", "Just a body.\n")

	if tech.ID != "plain" {
		t.Errorf("ID = %q, want plain (from the file name)", tech.ID)
	}
	if !strings.Contains(tech.Body.EN, "Just a body") {
		t.Errorf("Body.EN = %q", tech.Body.EN)
	}
}

func TestParseRejectsEmptyBody(t *testing.T) {
	_, err := Parse("web/empty.md", []byte("---\nid: empty\ntitle_en: Empty\n---\n\n"))
	if err == nil {
		t.Fatal("expected an error for a file with no body in either language")
	}
}

func TestParseRejectsUnterminatedFrontmatter(t *testing.T) {
	_, err := Parse("web/broken.md", []byte("---\nid: broken\ntitle_en: Broken\n"))
	if err == nil {
		t.Fatal("expected an error for frontmatter that is never closed")
	}
}

func TestCarriageReturnsAreTolerated(t *testing.T) {
	// Content written on Windows must load exactly like content written on Linux.
	crlf := strings.ReplaceAll(sampleFile, "\n", "\r\n")
	tech := mustParse(t, "web/sample-technique.md", crlf)

	if !strings.Contains(tech.Body.ZH, "中文正文") {
		t.Errorf("CRLF file lost its Chinese body: %q", tech.Body.ZH)
	}
	if tech.Title.EN != "Sample Technique" {
		t.Errorf("CRLF file mangled the title: %q", tech.Title.EN)
	}
}

func TestCategoryComesFromDirectory(t *testing.T) {
	cases := map[string][]string{
		"web/sample-technique.md":     {"web"},
		"offensive/web/x.md":          {"offensive", "web"},
		"a/b/c/x.md":                  {"a", "b", "c"},
		"x.md":                        nil,
		"./offensive/credential/x.md": {"offensive", "credential"},
	}
	for path, want := range cases {
		tech := mustParse(t, path, sampleFile)
		if strings.Join(tech.Category, "/") != strings.Join(want, "/") {
			t.Errorf("category for %q = %v, want %v", path, tech.Category, want)
		}
	}
}

func TestLangTextFallsBack(t *testing.T) {
	onlyEN := LangText{EN: "english"}
	got, served := onlyEN.Get(LangZH)
	if got != "english" || served != LangEN {
		t.Errorf("Get(zh) = (%q, %q), want the English text with served=en", got, served)
	}

	both := LangText{EN: "english", ZH: "中文"}
	if got, served := both.Get(LangZH); got != "中文" || served != LangZH {
		t.Errorf("Get(zh) = (%q, %q), want the Chinese text", got, served)
	}
}

func TestNormalizeLang(t *testing.T) {
	for _, in := range []string{"zh", "ZH", "zh-CN", "cn", "中文"} {
		if got := NormalizeLang(in); got != LangZH {
			t.Errorf("NormalizeLang(%q) = %q, want zh", in, got)
		}
	}
	for _, in := range []string{"en", "EN", "english", "", "klingon"} {
		if got := NormalizeLang(in); got != LangEN {
			t.Errorf("NormalizeLang(%q) = %q, want en", in, got)
		}
	}
}

// ------------------------------------------------------------------- loading

func mapFS(files map[string]string) fstest.MapFS {
	mfs := make(fstest.MapFS, len(files))
	for name, body := range files {
		mfs[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return mfs
}

const plainEN = `---
id: %s
title_en: %s
---

<!-- lang:en -->
body for %s
`

func TestLoadWalksTopicsAndSkipsParkedFiles(t *testing.T) {
	mfs := mapFS(map[string]string{
		"topics/offensive/web/one.md":   strings.ReplaceAll(plainEN, "%s", "one"),
		"topics/offensive/creds/two.md": strings.ReplaceAll(plainEN, "%s", "two"),
		"topics/_drafts/wip.md":         strings.ReplaceAll(plainEN, "%s", "wip"),
		"topics/.hidden/nope.md":        strings.ReplaceAll(plainEN, "%s", "nope"),
		"topics/offensive/_scratch.md":  strings.ReplaceAll(plainEN, "%s", "scratch"),
		"topics/offensive/notes.txt":    "not markdown",
		"topics/embed.go":               "package content",
	})

	lib, err := Load(mfs, "topics")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if lib.Len() != 2 {
		t.Fatalf("loaded %d techniques, want 2 (parked files and non-markdown must be skipped)", lib.Len())
	}
	if _, ok := lib.Get("one"); !ok {
		t.Error("technique one not found by id")
	}
	if _, ok := lib.Get("wip"); ok {
		t.Error("a file under _drafts/ was loaded")
	}
	if got := lib.Techniques[0].CategoryPath(); got != "offensive/creds" {
		t.Errorf("first technique category = %q", got)
	}
}

func TestLoadReportsDuplicateIDWithoutDroppingTheRest(t *testing.T) {
	dup := `---
id: same
title_en: Dup
---

<!-- lang:en -->
body
`
	mfs := mapFS(map[string]string{
		"topics/a/same.md": dup,
		"topics/b/same.md": dup,
		"topics/c/other.md": `---
id: other
title_en: Other
---

<!-- lang:en -->
body
`,
	})

	lib, err := Load(mfs, "topics")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if lib.Len() != 2 {
		t.Errorf("loaded %d techniques, want 2", lib.Len())
	}
	if len(lib.Errors()) != 1 {
		t.Errorf("got %d load errors, want 1 duplicate-id error: %v", len(lib.Errors()), lib.Errors())
	}
}

func TestLoadReportsBrokenFileAndKeepsGoing(t *testing.T) {
	mfs := mapFS(map[string]string{
		"topics/a/good.md":   strings.ReplaceAll(plainEN, "%s", "good"),
		"topics/a/broken.md": "---\nid: broken\ntitle_en: Broken\n",
	})

	lib, err := Load(mfs, "topics")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if lib.Len() != 1 {
		t.Errorf("loaded %d techniques, want 1 despite the broken file", lib.Len())
	}
	if len(lib.Errors()) != 1 {
		t.Errorf("got %d load errors, want 1", len(lib.Errors()))
	}
}

func TestLibraryCategoriesAndCounts(t *testing.T) {
	mfs := mapFS(map[string]string{
		"topics/offensive/web/a.md":   strings.ReplaceAll(plainEN, "%s", "a"),
		"topics/offensive/web/b.md":   strings.ReplaceAll(plainEN, "%s", "b"),
		"topics/offensive/creds/c.md": strings.ReplaceAll(plainEN, "%s", "c"),
	})

	lib, err := Load(mfs, "topics")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cats := lib.Categories()
	want := []string{"offensive", "offensive/creds", "offensive/web"}
	if strings.Join(cats, ",") != strings.Join(want, ",") {
		t.Errorf("Categories() = %v, want %v", cats, want)
	}
	// The web panel walks this list once and infers which rows have a sub-tree
	// from what follows them, so a parent's children have to come directly
	// after it. A category with children that is not followed by one of them
	// would render as a leaf.
	for i, c := range cats {
		hasChild := i+1 < len(cats) && strings.HasPrefix(cats[i+1], c+"/")
		if hasChild {
			continue
		}
		for _, other := range cats {
			if strings.HasPrefix(other, c+"/") {
				t.Errorf("Categories() = %v: %q has children but %q follows it, want one of its children", cats, c, cats[i+1])
				break
			}
		}
	}
	if got := lib.CountInCategory("offensive/web"); got != 2 {
		t.Errorf("CountInCategory(offensive/web) = %d, want 2", got)
	}
	if got := lib.CountInCategory("offensive"); got != 3 {
		t.Errorf("CountInCategory(offensive) = %d, want 3", got)
	}
}

// -------------------------------------------------------------------- search

func sampleLibrary(t *testing.T) *Library {
	t.Helper()
	mfs := mapFS(map[string]string{
		"topics/offensive/credential-access/sample-technique.md": sampleFile,
		"topics/offensive/web/other.md": `---
id: other
title_en: Other Thing
title_zh: 另一个东西
tags: [web]
tools: [ffuf]
attck: [T9999]
platform: [windows]
difficulty: advanced
---

<!-- lang:en -->
A page about cloud metadata and SSRF.

<!-- lang:zh -->
讲云元数据与 SSRF 的页面。
`,
	})
	lib, err := Load(mfs, "topics")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return lib
}

func ids(matches []Match) []string {
	out := make([]string, len(matches))
	for i, m := range matches {
		out[i] = m.Technique.ID
	}
	return out
}

func TestSearchFreeTermsAreAnded(t *testing.T) {
	lib := sampleLibrary(t)

	if got := ids(lib.Search("kerberos", 0)); len(got) != 1 || got[0] != "sample-technique" {
		t.Errorf("search kerberos = %v", got)
	}
	if got := ids(lib.Search("kerberos relays", 0)); len(got) != 1 {
		t.Errorf("AND of two present terms = %v, want one hit", got)
	}
	if got := ids(lib.Search("kerberos zzzznotpresent", 0)); len(got) != 0 {
		t.Errorf("AND with a missing term = %v, want no hits", got)
	}
}

func TestSearchIsCaseInsensitive(t *testing.T) {
	lib := sampleLibrary(t)
	if got := ids(lib.Search("KERBEROS", 0)); len(got) != 1 {
		t.Errorf("uppercase query = %v, want a hit", got)
	}
}

func TestSearchFindsChineseText(t *testing.T) {
	lib := sampleLibrary(t)

	// Both of these only occur in the Chinese bodies.
	if got := ids(lib.Search("中继", 0)); len(got) != 1 || got[0] != "sample-technique" {
		t.Errorf("chinese query 中继 = %v", got)
	}
	if got := ids(lib.Search("云元数据", 0)); len(got) != 1 || got[0] != "other" {
		t.Errorf("chinese query 云元数据 = %v", got)
	}
}

func TestSearchFieldConstraints(t *testing.T) {
	lib := sampleLibrary(t)

	cases := []struct {
		query string
		want  []string
	}{
		{"id:sample-technique", []string{"sample-technique"}},
		{"category:offensive", []string{"other", "sample-technique"}},
		{"category:offensive/web", []string{"other"}},
		{"category:offensive/credential-access", []string{"sample-technique"}},
		{"category:nope", nil},
		{"tag:web", []string{"other", "sample-technique"}},
		{"tag:nosuchtag", nil},
		{"tool:curl", []string{"sample-technique"}},
		{"attck:T1234", []string{"sample-technique"}},
		{"attck:T1234.001", []string{"sample-technique"}},
		{"platform:windows", []string{"other"}},
		{"difficulty:beginner", []string{"sample-technique"}},
		{"category:offensive relays", []string{"sample-technique"}},
		{"tag:web category:nope", nil},
	}

	for _, c := range cases {
		got := ids(lib.Search(c.query, 0))
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("Search(%q) = %v, want %v", c.query, got, c.want)
		}
	}
}

func TestSearchUnknownPrefixStaysFreeText(t *testing.T) {
	lib := sampleLibrary(t)

	// "foo:kerberos" is not a field we know, so it must not vanish: it is kept
	// as free text and simply finds nothing because of the literal prefix.
	if got := lib.Search("foo:kerberos", 0); len(got) != 0 {
		t.Errorf("unknown prefix = %v, want no hits (kept as literal text)", got)
	}
	// The bare term still works, proving the token was not dropped silently.
	if got := ids(lib.Search("kerberos", 0)); len(got) != 1 {
		t.Errorf("bare term = %v", got)
	}
}

func TestSearchTiesAreBrokenByIdForStability(t *testing.T) {
	lib := sampleLibrary(t)
	first := ids(lib.Search("category:offensive", 0))
	second := ids(lib.Search("category:offensive", 0))
	if strings.Join(first, ",") != strings.Join(second, ",") {
		t.Errorf("repeated identical queries returned different orders: %v vs %v", first, second)
	}
}

func TestSearchLimitAndEmptyQuery(t *testing.T) {
	lib := sampleLibrary(t)

	if got := lib.Search("", 0); len(got) != 2 {
		t.Errorf("empty query returned %d entries, want all 2", len(got))
	}
	if got := lib.Search("category:offensive", 1); len(got) != 1 {
		t.Errorf("limit=1 returned %d entries", len(got))
	}
}

// ------------------------------------------------------------------ validate

func TestValidateFlagsIncompleteEntry(t *testing.T) {
	lib := sampleLibrary(t)
	issues := lib.Validate()

	var missingSummary int
	for _, is := range issues {
		if is.Level == "error" {
			t.Errorf("unexpected error-level issue: %+v", is)
		}
		switch {
		case is.Path == "offensive/credential-access/sample-technique.md":
			// That entry carries every field, so it must come back completely clean.
			t.Errorf("a complete entry produced a warning: %+v", is)
		case is.Path == "offensive/web/other.md" && strings.Contains(is.Msg, "summary_en"):
			missingSummary++
		}
	}
	if missingSummary != 1 {
		t.Errorf("expected exactly one summary warning for other.md, got %d (issues: %+v)", missingSummary, issues)
	}

	errs, warns := CountIssues(issues)
	if errs != 0 || warns == 0 {
		t.Errorf("CountIssues = (%d errors, %d warnings), want (0, >0)", errs, warns)
	}
}

func TestParseExplainsColonInFrontmatterValues(t *testing.T) {
	// The single easiest frontmatter mistake: a value with a bare ": " in it.
	_, err := Parse("web/x.md", []byte(
		"---\nid: x\ntitle_en: A title with a colon: inside it\n---\n\n<!-- lang:en -->\nbody\n"))
	if err == nil {
		t.Fatal("expected an unquoted colon in frontmatter to fail the parse")
	}
	if !strings.Contains(err.Error(), `": "`) {
		t.Errorf("the error should explain the colon problem to a contributor, got: %v", err)
	}
}

func TestValidateSurfacesLoadErrorsAsErrors(t *testing.T) {
	mfs := mapFS(map[string]string{
		"topics/a/broken.md": "---\nid: broken\ntitle_en: Broken\n",
	})
	lib, err := Load(mfs, "topics")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	issues := lib.Validate()
	errs, _ := CountIssues(issues)
	if errs == 0 {
		t.Errorf("a file that failed to parse must show up as an error, got %+v", issues)
	}
}
