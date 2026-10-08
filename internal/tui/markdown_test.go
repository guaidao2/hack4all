package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// strip removes escape sequences so a test can assert on what the reader sees.
func strip(s string) string { return ansi.Strip(s) }

func TestRenderMarkdownRemovesHeadingHashes(t *testing.T) {
	got := strip(renderMarkdown("### Why it works\n", 60))

	if strings.Contains(got, "#") {
		t.Errorf("heading markers survived rendering: %q", got)
	}
	if !strings.Contains(got, "Why it works") {
		t.Errorf("heading text lost: %q", got)
	}
}

func TestRenderMarkdownUnwrapsCodeFences(t *testing.T) {
	md := "```bash\nimpacket-GetUserSPNs -request\n```\n"
	got := strip(renderMarkdown(md, 60))

	if strings.Contains(got, "```") {
		t.Errorf("code fence survived: %q", got)
	}
	if !strings.Contains(got, "impacket-GetUserSPNs -request") {
		t.Errorf("code content lost: %q", got)
	}
	// The language tag is kept, but as a label rather than as syntax.
	if !strings.Contains(got, "bash") {
		t.Errorf("language label lost: %q", got)
	}
}

func TestRenderMarkdownFormatsLists(t *testing.T) {
	got := strip(renderMarkdown("- first item\n- second item\n\n1. ordered one\n", 60))

	if !strings.Contains(got, "• first item") {
		t.Errorf("unordered list not turned into bullets: %q", got)
	}
	if !strings.Contains(got, "1. ordered one") {
		t.Errorf("ordered list marker lost: %q", got)
	}
}

func TestRenderMarkdownRendersInlineEmphasis(t *testing.T) {
	got := strip(renderMarkdown("Call `hashcat -m 13100` and **always** attribute.\n", 70))

	if strings.Contains(got, "`") || strings.Contains(got, "**") {
		t.Errorf("inline markers survived: %q", got)
	}
	if !strings.Contains(got, "hashcat -m 13100") || !strings.Contains(got, "always") {
		t.Errorf("inline text lost: %q", got)
	}
}

func TestRenderMarkdownTurnsLinksIntoReadableText(t *testing.T) {
	got := strip(renderMarkdown("See [the advisory](https://example.com/a).\n", 70))

	if !strings.Contains(got, "the advisory (https://example.com/a)") {
		t.Errorf("link not rendered readably: %q", got)
	}
}

func TestRenderMarkdownAlignsTablesIntoColumns(t *testing.T) {
	md := "| etype | Name | Mode |\n" +
		"|-------|------|------|\n" +
		"| 23 | RC4-HMAC | 13100 |\n" +
		"| 17 | AES128 | 19600 |\n"

	got := strip(renderMarkdown(md, 80))
	lines := strings.Split(got, "\n")

	var dataLines []string
	for _, l := range lines {
		if strings.Contains(l, "RC4-HMAC") || strings.Contains(l, "AES128") {
			dataLines = append(dataLines, l)
		}
	}
	if len(dataLines) != 2 {
		t.Fatalf("expected 2 table rows, got %d in %q", len(dataLines), got)
	}
	if !strings.Contains(dataLines[0], "│") {
		t.Errorf("table row has no column separator: %q", dataLines[0])
	}
	// Both cells must start at the same column, i.e. the rows are aligned.
	if a, b := strings.Index(dataLines[0], "RC4-HMAC"), strings.Index(dataLines[1], "AES128"); a != b {
		t.Errorf("values are not aligned: %d vs %d\n%q\n%q", a, b, dataLines[0], dataLines[1])
	}
	if strings.Contains(got, "|---") {
		t.Errorf("the markdown separator row was rendered literally: %q", got)
	}
}

func TestRenderMarkdownKeepsEverythingInsideTheWidth(t *testing.T) {
	md := "### Step 1 — Find accounts with SPNs\n\n" +
		"From Linux with a valid credential you can ask the KDC for a service ticket " +
		"and it will hand one back without checking whether you are allowed to use it.\n\n" +
		"```bash\n" +
		"impacket-GetUserSPNs corp.local/jdoe:'Summer2026!' -dc-ip 10.10.10.10 -request\n" +
		"```\n\n" +
		"| etype | Name | Hashcat mode |\n|---|---|---|\n| 23 | RC4-HMAC | 13100 |\n"

	const width = 40
	got := renderMarkdown(md, width)

	for _, l := range strings.Split(got, "\n") {
		if w := ansi.StringWidth(l); w > width {
			t.Errorf("rendered line is %d cells wide (limit %d): %q", w, width, strip(l))
		}
	}
}

func TestRenderMarkdownSurvivesMalformedInput(t *testing.T) {
	// Unterminated fence, dangling table row, empty input: none may panic, hang
	// or index out of range. An empty result is fine — an empty quote is not
	// content.
	for _, md := range []string{
		"```bash\nno closing fence\n",
		"| a | b |\n",
		"|",
		"",
		">",
		"#",
		"- ",
		"|---|\n|---|\n",
	} {
		_ = renderMarkdown(md, 30)
	}
}

func TestWrapKeepsClosingPunctuationOffLineStarts(t *testing.T) {
	text := "这是一个很长的中文句子，用来测试折行的时候标点会不会落在行首，如果不做处理的话逗号和句号就会跑到下一行的开头。"

	lines := renderInlineWrapped(text, 20)
	if len(lines) < 2 {
		t.Fatalf("expected the text to wrap, got %q", lines)
	}
	for _, l := range lines {
		rs := []rune(l)
		if len(rs) == 0 {
			continue
		}
		if isClosingPunct(rs[0]) {
			t.Errorf("line opens with closing punctuation: %q", l)
		}
	}
}

func TestRenderMarkdownRendersQuotes(t *testing.T) {
	got := strip(renderMarkdown("> keep this in mind\n", 60))

	if !strings.Contains(got, "keep this in mind") {
		t.Errorf("quote text lost: %q", got)
	}
	if !strings.Contains(got, "│") {
		t.Errorf("quote has no left bar: %q", got)
	}
}
