package core

import (
	"fmt"
	"strings"
)

// Plain renders a technique as terminal text: no ANSI colour, no Markdown
// reflow. It is what `hack4all -x` prints, so it must stay readable both for a
// human in a terminal and for a tool that captures stdout.
func (t *Technique) Plain(lang string) string {
	lang = NormalizeLang(lang)

	title, _ := t.Title.Get(lang)
	summary, _ := t.Summary.Get(lang)
	body, served := t.Body.Get(lang)

	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n")

	meta := make([]string, 0, 4)
	if c := t.CategoryPath(); c != "" {
		meta = append(meta, c)
	}
	if t.Difficulty != "" {
		meta = append(meta, t.Difficulty)
	}
	if len(t.ATTACK) > 0 {
		meta = append(meta, strings.Join(t.ATTACK, " "))
	}
	if len(meta) > 0 {
		b.WriteString(strings.Join(meta, "  ·  "))
		b.WriteString("\n")
	}
	b.WriteString(strings.Repeat("─", 72))
	b.WriteString("\n")

	if len(t.Tags) > 0 {
		fmt.Fprintf(&b, "tags  : %s\n", strings.Join(t.Tags, ", "))
	}
	if len(t.Tools) > 0 {
		fmt.Fprintf(&b, "tools : %s\n", strings.Join(t.Tools, ", "))
	}
	if len(t.Platform) > 0 {
		fmt.Fprintf(&b, "target: %s\n", strings.Join(t.Platform, ", "))
	}
	if t.Updated != "" {
		fmt.Fprintf(&b, "updated: %s\n", t.Updated)
	}
	if served != lang {
		fmt.Fprintf(&b, "note  : %s text not available, showing %s\n", lang, served)
	}

	if summary != "" {
		b.WriteString("\n")
		b.WriteString(summary)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}

	return b.String()
}

// Row renders a one-line entry for list output: the id, the category and the
// title in the requested language.
func (t *Technique) Row(lang string) string {
	title, _ := t.Title.Get(lang)
	cat := t.CategoryPath()
	if cat == "" {
		cat = "-"
	}
	return fmt.Sprintf("%-34s %-42s %s", t.ID, cat, title)
}

// RowHeader is the header matching Row's columns.
func RowHeader() string {
	return fmt.Sprintf("%-34s %-42s %s", "ID", "CATEGORY", "TITLE")
}
