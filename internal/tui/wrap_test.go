package tui

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

func TestWrapLineBreaksLongEnglishText(t *testing.T) {
	line := "The KDC will hand out a TGS to any authenticated user who asks for it"

	got := wrapLine(line, 30)
	if len(got) < 2 {
		t.Fatalf("expected the line to wrap, got %d line(s): %q", len(got), got)
	}
	for _, l := range got {
		if w := runewidth.StringWidth(l); w > 30 {
			t.Errorf("wrapped line %q is %d cells wide, want <= 30", l, w)
		}
	}
	// Wrapping must not lose or split a word.
	if joinWords(got) != joinWords([]string{line}) {
		t.Errorf("wrapping changed the text:\n got %q\nwant %q", joinWords(got), line)
	}
}

func TestWrapLineBreaksChineseByCharacter(t *testing.T) {
	line := "KDC 会把 TGS 发给任何提出请求的已认证用户"

	got := wrapLine(line, 12)
	if len(got) < 2 {
		t.Fatalf("expected Chinese text to wrap, got %q", got)
	}
	for _, l := range got {
		if w := runewidth.StringWidth(l); w > 12 {
			t.Errorf("line %q is %d cells wide, want <= 12", l, w)
		}
	}
	if joined := strings.Join(got, ""); !strings.Contains(joined, "已认证用户") {
		t.Errorf("content lost while wrapping Chinese: %q", got)
	}
}

func TestWrapLineKeepsShortLines(t *testing.T) {
	if got := wrapLine("short", 40); len(got) != 1 || got[0] != "short" {
		t.Errorf("wrapLine(short) = %q, want it unchanged", got)
	}
}

func TestWrapLinePreservesIndent(t *testing.T) {
	got := wrapLine("    - a fairly long list item that has to wrap somewhere here", 24)
	if len(got) < 2 {
		t.Fatalf("expected the list item to wrap, got %q", got)
	}
	for _, l := range got {
		if !strings.HasPrefix(l, "    ") {
			t.Errorf("continuation lost its indent: %q", l)
		}
	}
}

func TestWrapLineHandlesUnbreakableToken(t *testing.T) {
	// A long URL or command with no spaces has to be hard-split, not dropped.
	long := "https://example.com/" + strings.Repeat("a", 60)
	got := wrapLine(long, 24)
	for _, l := range got {
		if w := runewidth.StringWidth(l); w > 24 {
			t.Errorf("line %q is %d cells wide, want <= 24", l, w)
		}
	}
	if len(strings.Join(got, "")) != len(long) {
		t.Errorf("hard-splitting lost characters: %d in, %d out", len(long), len(strings.Join(got, "")))
	}
}

func TestWrapTextLeavesRulesAlone(t *testing.T) {
	rule := strings.Repeat("─", 72)
	out := wrapText("title\n"+rule+"\nbody", 40)

	if !strings.Contains(out, rule) {
		t.Error("a horizontal rule was wrapped or altered")
	}
}

func TestWrapTextNeverExceedsWidth(t *testing.T) {
	in := "### Why it works\n\n" +
		"A long paragraph that definitely exceeds the width we wrap it to, and then keeps going for a while.\n\n" +
		"```bash\n" +
		"impacket-GetUserSPNs corp.local/jdoe:'Summer2026!' -dc-ip 10.10.10.10 -request\n" +
		"```\n"

	const width = 34
	out := wrapText(in, width)

	for _, l := range strings.Split(out, "\n") {
		if isRule(strings.TrimSpace(l)) {
			continue
		}
		if w := runewidth.StringWidth(l); w > width {
			t.Errorf("line %q is %d cells wide, want <= %d", l, w, width)
		}
	}
	// Nothing may be dropped: the long command has to survive in full.
	if flat := strings.ReplaceAll(out, "\n", ""); !strings.Contains(flat, "impacket-GetUserSPNs") {
		t.Error("wrapping dropped part of the command")
	}
}

func TestTruncateCountsCJKAsTwoCells(t *testing.T) {
	got := truncate("中文标题中文标题", 8)
	if w := runewidth.StringWidth(got); w > 8 {
		t.Errorf("truncate produced %d cells: %q", w, got)
	}
	if got := truncate("abcdef", 4); runewidth.StringWidth(got) > 4 {
		t.Errorf("truncate(abcdef, 4) = %q", got)
	}
	if got := truncate("abc", 10); got != "abc" {
		t.Errorf("truncate shortened a string that already fits: %q", got)
	}
}

func TestIsRule(t *testing.T) {
	for _, s := range []string{"────", "----", "====", "____"} {
		if !isRule(s) {
			t.Errorf("isRule(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "---", "a---", "- - -", "中文正文"} {
		if isRule(s) {
			t.Errorf("isRule(%q) = true, want false", s)
		}
	}
}

func joinWords(lines []string) string {
	return strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
}
