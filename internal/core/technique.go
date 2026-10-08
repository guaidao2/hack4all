// Package core holds the Hack4all knowledge base: the technique model, the
// loader and the search index.
//
// Every entry point (TUI, local web UI, non-interactive CLI) goes through this
// package, so the three front-ends can never disagree about what the content
// says. Adding a front-end must not require touching the content layer.
package core

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Supported languages of the knowledge base.
const (
	LangEN = "en"
	LangZH = "zh"
)

// DefaultLang is used when a caller does not ask for a specific language.
// English is the primary language of the knowledge base, Chinese the parallel one.
const DefaultLang = LangEN

// NormalizeLang maps loose user input (en, EN, english, zh, zh-CN, 中文, …)
// onto a supported language code. Anything unknown becomes the default.
func NormalizeLang(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "zh", "zh-cn", "zh-hans", "zh-hant", "cn", "chinese", "中文", "汉语", "简体中文":
		return LangZH
	default:
		return LangEN
	}
}

// =============================================================================
// Bilingual text
// =============================================================================

// LangText stores one value per language.
type LangText struct {
	EN string `json:"en"`
	ZH string `json:"zh"`
}

// Get returns the text for the requested language. When that language is empty
// it falls back to the other one, because half a technique is better than none.
// The second return value is the language actually served.
func (t LangText) Get(lang string) (string, string) {
	if NormalizeLang(lang) == LangZH {
		if strings.TrimSpace(t.ZH) != "" {
			return t.ZH, LangZH
		}
		return t.EN, LangEN
	}
	if strings.TrimSpace(t.EN) != "" {
		return t.EN, LangEN
	}
	return t.ZH, LangZH
}

// Has reports whether the given language has any content at all.
func (t LangText) Has(lang string) bool {
	if NormalizeLang(lang) == LangZH {
		return strings.TrimSpace(t.ZH) != ""
	}
	return strings.TrimSpace(t.EN) != ""
}

// =============================================================================
// Technique
// =============================================================================

// Technique is one knowledge-base entry: a single technique, attack or bug
// class, written in both languages inside one Markdown file.
type Technique struct {
	ID         string   `json:"id"`
	Title      LangText `json:"title"`
	Summary    LangText `json:"summary"`
	Body       LangText `json:"body"`
	Category   []string `json:"category"`
	Tags       []string `json:"tags"`
	Tools      []string `json:"tools"`
	ATTACK     []string `json:"attck"`
	Platform   []string `json:"platform"`
	Difficulty string   `json:"difficulty"`
	Updated    string   `json:"updated"`
	Path       string   `json:"path"`

	// index caches the lowercase haystacks used by Search. Never serialised.
	index map[string]string
}

// CategoryPath renders the directory-derived category as a single string.
func (t *Technique) CategoryPath() string {
	return strings.Join(t.Category, "/")
}

// TitleFor returns the title in the requested language, with fallback.
func (t *Technique) TitleFor(lang string) string {
	s, _ := t.Title.Get(lang)
	return s
}

// BuildIndex refreshes the lowercase search haystacks. Called by Parse, and
// available for callers that mutate a Technique in memory.
func (t *Technique) BuildIndex() {
	t.index = map[string]string{
		// The id is indexed as well. It is derived from the filename, so people
		// search for it — "classical-ciphers", "red-team-tactical-map" — without
		// thinking about the id: prefix.
		"id":    strings.ToLower(t.ID),
		"title": strings.ToLower(t.Title.EN + " " + t.Title.ZH),
		"meta": strings.ToLower(strings.Join([]string{
			strings.Join(t.Tags, " "),
			strings.Join(t.Tools, " "),
			strings.Join(t.ATTACK, " "),
			strings.Join(t.Platform, " "),
			strings.Join(t.Category, " "),
			t.Difficulty,
		}, " ")),
		"summary": strings.ToLower(t.Summary.EN + " " + t.Summary.ZH),
		"body":    strings.ToLower(t.Body.EN + " " + t.Body.ZH),
	}
}

// =============================================================================
// Parsing
// =============================================================================

// frontmatter mirrors the YAML header of a technique file. Keys are flat on
// purpose: contributors edit these by hand and should not have to remember
// nested YAML.
type frontmatter struct {
	ID         string   `yaml:"id"`
	TitleEN    string   `yaml:"title_en"`
	TitleZH    string   `yaml:"title_zh"`
	SummaryEN  string   `yaml:"summary_en"`
	SummaryZH  string   `yaml:"summary_zh"`
	Tags       []string `yaml:"tags"`
	Tools      []string `yaml:"tools"`
	ATTACK     []string `yaml:"attck"`
	Platform   []string `yaml:"platform"`
	Difficulty string   `yaml:"difficulty"`
	Updated    string   `yaml:"updated"`
}

// langMarkRe matches the separator comments that split the bilingual body:
//
//	<!-- lang:en -->
//	<!-- lang:zh -->
var langMarkRe = regexp.MustCompile(`(?m)^[ \t]*<!--[ \t]*lang:(en|zh)[ \t]*-->[ \t]*$`)

// Parse turns one Markdown file into a Technique. relPath is the path relative
// to the content root; it decides the category and the fallback ID.
func Parse(relPath string, data []byte) (*Technique, error) {
	fmRaw, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}

	var fm frontmatter
	if len(fmRaw) > 0 {
		if err := yaml.Unmarshal(fmRaw, &fm); err != nil {
			return nil, fmt.Errorf("frontmatter: %w%s", err, yamlHint(err))
		}
	}

	if dup := duplicateLangMark(body); dup != "" {
		return nil, fmt.Errorf("duplicate <!-- lang:%s --> marker: each language must appear exactly once", dup)
	}
	langs := splitLanguages(body)
	t := &Technique{
		ID:         strings.TrimSpace(fm.ID),
		Title:      LangText{EN: strings.TrimSpace(fm.TitleEN), ZH: strings.TrimSpace(fm.TitleZH)},
		Summary:    LangText{EN: strings.TrimSpace(fm.SummaryEN), ZH: strings.TrimSpace(fm.SummaryZH)},
		Body:       LangText{EN: langs[LangEN], ZH: langs[LangZH]},
		Category:   categoryFromPath(relPath),
		Tags:       cleanList(fm.Tags),
		Tools:      cleanList(fm.Tools),
		ATTACK:     cleanList(fm.ATTACK),
		Platform:   cleanList(fm.Platform),
		Difficulty: strings.TrimSpace(fm.Difficulty),
		Updated:    strings.TrimSpace(fm.Updated),
		Path:       relPath,
	}

	if t.ID == "" {
		base := path.Base(relPath)
		t.ID = strings.TrimSuffix(base, path.Ext(base))
	}
	if t.Title.EN == "" && t.Title.ZH == "" {
		t.Title.EN = t.ID
	}
	if t.Body.EN == "" && t.Body.ZH == "" {
		return nil, fmt.Errorf("empty body: no text before or after the language markers")
	}

	t.BuildIndex()
	return t, nil
}

// yamlHint turns the most common frontmatter mistake into advice.
//
// A bare ": " inside a value makes YAML think a nested mapping begins, and the
// parser's own message ("mapping values are not allowed in this context") says
// nothing about what a contributor should do differently.
func yamlHint(err error) string {
	if err != nil && strings.Contains(err.Error(), "mapping values are not allowed") {
		return ` — a frontmatter value contains ": ", which YAML reads as a nested key; rephrase it or wrap the value in quotes`
	}
	return ""
}

// splitFrontmatter separates an optional leading YAML block from the body.
// A file without frontmatter is valid; the whole file is then the body.
func splitFrontmatter(data []byte) ([]byte, string, error) {
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return nil, s, nil
	}
	rest := s[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, "", fmt.Errorf("frontmatter opened with --- but never closed")
	}
	fm := rest[:end]
	body := strings.TrimPrefix(rest[end+len("\n---"):], "\n")
	return []byte(fm), body, nil
}

// duplicateLangMark reports a language marker that appears more than once.
//
// A repeat is always a mistake, and a silent one: splitLanguages assigns per
// language, so a second "<!-- lang:zh -->" with text after it replaces the first
// Chinese section instead of adding to it. Quietly losing half an entry is worse
// than refusing to load the file.
func duplicateLangMark(body string) string {
	counts := map[string]int{}
	for _, m := range langMarkRe.FindAllStringSubmatch(body, -1) {
		counts[m[1]]++
		if counts[m[1]] > 1 {
			return m[1]
		}
	}
	return ""
}

// splitLanguages splits the body on the language markers. A body with no
// marker at all is treated as English.
func splitLanguages(body string) map[string]string {
	out := make(map[string]string, 2)
	locs := langMarkRe.FindAllStringSubmatchIndex(body, -1)
	if len(locs) == 0 {
		if strings.TrimSpace(body) != "" {
			out[LangEN] = strings.TrimSpace(body)
		}
		return out
	}
	for i, loc := range locs {
		lang := body[loc[2]:loc[3]]
		start := loc[1]
		end := len(body)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		if text := strings.TrimSpace(body[start:end]); text != "" {
			out[lang] = text
		}
	}
	return out
}

// categoryFromPath derives the category tree from the directory layout, e.g.
// offensive/credential-access/kerberoasting.md -> [offensive credential-access].
// The directory is the single source of truth for categories: they are never
// duplicated into frontmatter, so the two can never drift apart.
func categoryFromPath(relPath string) []string {
	dir := path.Dir(path.Clean(strings.TrimPrefix(relPath, "./")))
	if dir == "." || dir == "/" || dir == "" {
		return nil
	}
	parts := strings.Split(dir, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" && !strings.HasPrefix(p, ".") {
			out = append(out, p)
		}
	}
	return out
}

func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
