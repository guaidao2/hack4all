package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// =============================================================================
// Terminal Markdown rendering
// =============================================================================

// renderMarkdown turns a technique body into styled terminal text: headings lose
// their hashes, code fences become indented blocks, tables are aligned into real
// columns, list markers become bullets and inline emphasis becomes colour.
func renderMarkdown(md string, width int) string {
	if width < 16 {
		width = 16
	}

	lines := strings.Split(md, "\n")
	out := make([]string, 0, len(lines))

	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == "":
			out = append(out, "")
			i++

		case strings.HasPrefix(trimmed, "```"):
			lang := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			i++
			var code []string
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				code = append(code, lines[i])
				i++
			}
			if i < len(lines) {
				i++ // consume the closing fence
			}
			out = append(out, renderCodeBlock(code, width, lang)...)

		case isTableRow(trimmed):
			rows, consumed := parseTable(lines[i:])
			if len(rows) == 0 {
				// A lone pipe that is not a table: render it as plain text.
				out = append(out, renderInlineWrapped(line, width)...)
				i++
				break
			}
			out = append(out, renderTable(rows, width)...)
			i += consumed

		case headingRe.MatchString(trimmed):
			m := headingRe.FindStringSubmatch(trimmed)
			out = append(out, renderHeading(m[2], len(m[1]), width)...)
			i++

		case isRule(trimmed):
			out = append(out, ruleStyle.Render(strings.Repeat("─", width)))
			i++

		case strings.HasPrefix(trimmed, ">"):
			var quote []string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), ">") {
				q := strings.TrimSpace(lines[i])
				quote = append(quote, strings.TrimSpace(strings.TrimPrefix(q, ">")))
				i++
			}
			out = append(out, renderQuote(strings.Join(quote, " "), width)...)

		case ulRe.MatchString(line) || olRe.MatchString(line):
			out = append(out, renderListItem(line, width)...)
			i++

		default:
			var para []string
			for i < len(lines) && !isBlockStart(lines, i) {
				para = append(para, strings.TrimSpace(lines[i]))
				i++
			}
			if len(para) == 0 { // defensive: never advance zero lines
				para = append(para, trimmed)
				i++
			}
			out = append(out, renderInlineWrapped(strings.Join(para, " "), width)...)
		}
	}

	return strings.Join(out, "\n")
}

// isBlockStart reports whether the line at index i begins a new Markdown block.
func isBlockStart(lines []string, i int) bool {
	t := strings.TrimSpace(lines[i])
	switch {
	case t == "",
		strings.HasPrefix(t, "```"),
		strings.HasPrefix(t, ">"),
		strings.HasPrefix(t, "|"),
		isRule(t),
		headingRe.MatchString(t),
		ulRe.MatchString(t),
		olRe.MatchString(t):
		return true
	}
	return false
}

// ------------------------------------------------------------------ headings

func renderHeading(text string, level, width int) []string {
	var style lipgloss.Style
	switch {
	case level <= 2:
		style = heading1Style
	case level == 3:
		style = heading2Style
	default:
		style = heading3Style
	}

	wrapped := wrapLines(text, width)
	out := make([]string, 0, len(wrapped))
	for _, l := range wrapped {
		out = append(out, style.Render(l))
	}
	return out
}

// ------------------------------------------------------------------ code

func renderCodeBlock(code []string, width int, lang string) []string {
	if len(code) == 0 {
		return nil
	}

	inner := width - 5 // block margin plus the style's own padding
	if inner < 10 {
		inner = 10
	}

	var wrapped []string
	for _, l := range code {
		wrapped = append(wrapped, wrapLines(l, inner)...)
	}

	out := make([]string, 0, len(wrapped)+2)
	if lang != "" {
		out = append(out, codeLangStyle.Render("  "+lang))
	}
	out = append(out, codeBlockStyle.Width(inner).Render(strings.Join(wrapped, "\n")))
	return out
}

// ------------------------------------------------------------------ tables

// parseTable reads a whole Markdown table starting at lines[0] and returns its
// rows plus how many source lines it consumed (the separator row included).
func parseTable(lines []string) ([][]string, int) {
	var rows [][]string
	i := 0
	for i < len(lines) {
		t := strings.TrimSpace(lines[i])
		if !isTableRow(t) {
			break
		}
		if !isTableSeparator(t) {
			rows = append(rows, splitTableRow(t))
		}
		i++
	}
	return rows, i
}

func isTableRow(t string) bool {
	return strings.HasPrefix(t, "|") && strings.Count(t, "|") >= 2
}

func isTableSeparator(t string) bool {
	if !isTableRow(t) {
		return false
	}
	body := strings.Trim(t, "| ")
	if body == "" {
		return false
	}
	for _, r := range body {
		if r != '-' && r != ':' && r != ' ' && r != '|' {
			return false
		}
	}
	return strings.ContainsRune(body, '-')
}

func splitTableRow(t string) []string {
	t = strings.TrimSpace(t)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	parts := strings.Split(t, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// renderTable aligns a table into real columns, wrapping any cell that does not
// fit instead of cutting it off — a truncated table is how payloads get lost.
func renderTable(rows [][]string, width int) []string {
	if len(rows) == 0 {
		return nil
	}

	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	if cols == 0 {
		return nil
	}
	for i := range rows {
		for len(rows[i]) < cols {
			rows[i] = append(rows[i], "")
		}
	}

	// Column widths come from the plain text: markup characters are not printed.
	widths := make([]int, cols)
	for _, r := range rows {
		for c, cell := range r {
			if w := runewidth.StringWidth(plainInline(cell)); w > widths[c] {
				widths[c] = w
			}
		}
	}

	const sep = " │ "
	sepWidth := runewidth.StringWidth(sep)
	avail := width - sepWidth*(cols-1)
	if minCols := cols * 8; avail < minCols {
		// A very narrow pane gets a table that overflows, rather than columns
		// too small to read a single word of.
		avail = minCols
	}

	// Shrink the widest column until the table fits the pane.
	for sum(widths) > avail {
		widest := 0
		for c := range widths {
			if widths[c] > widths[widest] {
				widest = c
			}
		}
		if widths[widest] <= 8 {
			break
		}
		widths[widest]--
	}

	var out []string
	for ri, r := range rows {
		cells := make([][]string, cols)
		height := 1
		for c := 0; c < cols; c++ {
			cells[c] = renderInlineWrapped(r[c], widths[c])
			if len(cells[c]) == 0 {
				cells[c] = []string{""}
			}
			if len(cells[c]) > height {
				height = len(cells[c])
			}
		}

		for li := 0; li < height; li++ {
			parts := make([]string, cols)
			for c := 0; c < cols; c++ {
				cell := ""
				if li < len(cells[c]) {
					cell = cells[c][li]
				}
				if pad := widths[c] - lipgloss.Width(cell); pad > 0 {
					cell += strings.Repeat(" ", pad)
				}
				parts[c] = cell
			}
			line := strings.Join(parts, sep)
			if ri == 0 {
				out = append(out, tableHeaderStyle.Render(line))
			} else {
				out = append(out, line)
			}
		}

		if ri == 0 {
			bars := make([]string, cols)
			for c := range bars {
				bars[c] = strings.Repeat("─", widths[c])
			}
			out = append(out, tableRuleStyle.Render(strings.Join(bars, "─┼─")))
		}
	}

	return out
}

// ------------------------------------------------------------------ lists, quotes

func renderListItem(line string, width int) []string {
	trimmed := strings.TrimSpace(line)

	marker, text := "•", ""
	switch m := ulRe.FindStringSubmatch(trimmed); {
	case m != nil:
		text = m[1]
	default:
		if m := olRe.FindStringSubmatch(trimmed); m != nil {
			marker, text = m[1]+".", m[2]
		} else {
			return renderInlineWrapped(line, width)
		}
	}

	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	prefix := indent + bulletStyle.Render(marker+" ")
	// Continuation lines line up under the text, not under the marker.
	contPad := strings.Repeat(" ", runewidth.StringWidth(marker)+1)
	avail := width - runewidth.StringWidth(prefix)
	if avail < 8 {
		avail = 8
	}

	wrapped := renderInlineWrapped(text, avail)
	out := make([]string, 0, len(wrapped))
	for i, l := range wrapped {
		if i == 0 {
			out = append(out, prefix+l)
		} else {
			out = append(out, indent+contPad+l)
		}
	}
	return out
}

func renderQuote(text string, width int) []string {
	prefix := quoteStyle.Render("│ ")
	avail := width - runewidth.StringWidth(prefix)
	if avail < 8 {
		avail = 8
	}

	wrapped := renderInlineWrapped(text, avail)
	out := make([]string, 0, len(wrapped))
	for _, l := range wrapped {
		out = append(out, prefix+l)
	}
	return out
}

// =============================================================================
// Inline styling that survives wrapping
// =============================================================================

// span is a run of text sharing one style.
type span struct {
	text  string
	style *lipgloss.Style
}

// piece is the smallest unit a line may break between: a word with its trailing
// space, or one wide (CJK) character.
type piece struct {
	text  string
	style *lipgloss.Style
}

var (
	headingRe = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	ulRe      = regexp.MustCompile(`^\s*[-*+]\s+(.*)$`)
	olRe      = regexp.MustCompile(`^\s*(\d+)\.\s+(.*)$`)

	// code | bold | italic | link. Italic delimiters must not hug whitespace,
	// which keeps a stray asterisk in prose ("svc_* and sql_*") from turning
	// everything between it and the next one into emphasis.
	inlineRe = regexp.MustCompile("`([^`]+)`|\\*\\*([^*]+)\\*\\*|\\*([^\\s*](?:[^*]*[^\\s*])?)\\*|\\[([^\\]]+)\\]\\(([^)\\s]+)\\)")
)

// parseInline splits a line into styled spans.
func parseInline(s string) []span {
	var spans []span
	idx := 0

	for _, m := range inlineRe.FindAllStringSubmatchIndex(s, -1) {
		if m[0] > idx {
			spans = append(spans, span{text: s[idx:m[0]]})
		}
		switch {
		case m[2] >= 0: // `code`
			st := inlineCodeStyle
			spans = append(spans, span{text: s[m[2]:m[3]], style: &st})
		case m[4] >= 0: // **bold**
			st := boldStyle
			spans = append(spans, span{text: s[m[4]:m[5]], style: &st})
		case m[6] >= 0: // *italic*
			st := italicStyle
			spans = append(spans, span{text: s[m[6]:m[7]], style: &st})
		default: // [text](url)
			spans = append(spans, span{text: s[m[8]:m[9]] + " (" + s[m[10]:m[11]] + ")"})
		}
		idx = m[1]
	}

	if idx < len(s) {
		spans = append(spans, span{text: s[idx:]})
	}
	return spans
}

// plainInline is the text as the reader will see it, markup removed. Needed for
// measuring: a code span is narrower than its backticks suggest.
func plainInline(s string) string {
	var b strings.Builder
	for _, sp := range parseInline(s) {
		b.WriteString(sp.text)
	}
	return b.String()
}

// tokenizeSpans flattens styled spans into break opportunities.
func tokenizeSpans(spans []span) []piece {
	var out []piece

	for _, sp := range spans {
		var word strings.Builder
		flush := func() {
			if word.Len() > 0 {
				out = append(out, piece{text: word.String(), style: sp.style})
				word.Reset()
			}
		}
		for _, r := range sp.text {
			switch {
			case r == ' ':
				word.WriteRune(r)
				flush()
			case runewidth.RuneWidth(r) == 2: // CJK: breakable anywhere
				flush()
				if isClosingPunct(r) && len(out) > 0 {
					// Keep closing punctuation glued to what precedes it, so a
					// wrapped line never opens with a comma or a full stop.
					out[len(out)-1].text += string(r)
					continue
				}
				out = append(out, piece{text: string(r), style: sp.style})
			default:
				word.WriteRune(r)
			}
		}
		flush()
	}

	return out
}

// renderInlineWrapped applies inline styles and wraps in one pass.
//
// The two cannot be separated: styling first makes the text unmeasurable, and
// wrapping first lets a span be cut in half, which is how a stray backtick ends
// up printed in the middle of a sentence.
func renderInlineWrapped(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	return fillLines(tokenizeSpans(parseInline(s)), width)
}

// fillLines greedily packs pieces into lines of at most width cells.
func fillLines(pieces []piece, width int) []string {
	var (
		lines []string
		cur   strings.Builder
		curW  int
	)

	flush := func() {
		lines = append(lines, strings.TrimRight(cur.String(), " "))
		cur.Reset()
		curW = 0
	}

	for _, p := range pieces {
		text := p.text
		if curW == 0 {
			text = strings.TrimLeft(text, " ")
		}
		if text == "" {
			continue
		}

		if curW > 0 && curW+runewidth.StringWidth(text) > width {
			flush()
			text = strings.TrimLeft(text, " ")
			if text == "" {
				continue
			}
		}

		// A single unit wider than the whole line (a long URL, a long run of
		// Chinese) has to be hard-split; it must still never be dropped.
		for runewidth.StringWidth(text) > width {
			head, tail := splitCells(text, width-curW)
			if head == "" {
				break
			}
			cur.WriteString(renderPiece(p.style, head))
			flush()
			text = tail
		}
		if text == "" {
			continue
		}

		cur.WriteString(renderPiece(p.style, text))
		curW += runewidth.StringWidth(text)
	}

	if strings.TrimSpace(cur.String()) != "" {
		flush()
	}
	return lines
}

// closingPunct are the marks that must never open a wrapped line. This is the
// Chinese 避头尾 (line-break prohibition) rule for closing punctuation: a comma
// or full stop stranded at the start of a line reads as a typo.
const closingPunct = "，。、；：？！）］｝》」』】〉·"

func isClosingPunct(r rune) bool {
	return strings.ContainsRune(closingPunct, r)
}

func renderPiece(style *lipgloss.Style, text string) string {
	if style == nil {
		return text
	}
	return style.Render(text)
}

// splitCells cuts s after at most n terminal cells.
func splitCells(s string, n int) (head, tail string) {
	if n <= 0 {
		return "", s
	}
	w := 0
	for i, r := range s {
		rw := runewidth.RuneWidth(r)
		if w+rw > n {
			return s[:i], s[i:]
		}
		w += rw
	}
	return s, ""
}

func sum(v []int) int {
	t := 0
	for _, n := range v {
		t += n
	}
	return t
}

// =============================================================================
// Markdown styles
// =============================================================================

// Text styles carry no background: the terminal's own transparency is the user's
// choice and must show through. The few backgrounds below are deliberate small
// patches of colour, not a screen fill.
var (
	heading1Style = lipgloss.NewStyle().Bold(true).Foreground(colAcc)
	heading2Style = lipgloss.NewStyle().Bold(true).Foreground(colAcc2)
	heading3Style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))

	ruleStyle        = lipgloss.NewStyle().Foreground(colLine)
	tableRuleStyle   = lipgloss.NewStyle().Foreground(colLine)
	tableHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(colAcc2)

	codeLangStyle  = lipgloss.NewStyle().Foreground(colDim)
	codeBlockStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("236")).
			Padding(0, 1).
			MarginLeft(2)

	bulletStyle = lipgloss.NewStyle().Foreground(colAcc)
	quoteStyle  = lipgloss.NewStyle().Foreground(colLine)

	boldStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	italicStyle = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("223"))

	inlineCodeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("222")).
			Background(lipgloss.Color("237"))
)
