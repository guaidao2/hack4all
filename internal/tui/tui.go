// Package tui is the interactive front-end: search on the left, the selected
// technique on the right, both fed by the shared core library.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/guaidao2/hack4all/internal/core"
)

// Run starts the interactive UI and blocks until the user quits.
func Run(lib *core.Library, lang string) error {
	if lib.Len() == 0 {
		return fmt.Errorf("the knowledge base is empty")
	}
	m := newModel(lib, lang)
	_, err := tea.NewProgram(m,
		tea.WithAltScreen(),
		// Wheel and clicks. Holding Shift still selects text in most terminals.
		tea.WithMouseCellMotion(),
	).Run()
	return err
}

// =============================================================================
// Model
// =============================================================================

type model struct {
	lib  *core.Library
	lang string

	input   textinput.Model
	query   string
	matches []core.Match
	cursor  int

	// listOffset is the index of the first visible list entry. It is kept apart
	// from the cursor so the pane can scroll before the selection reaches the
	// edge, leaving a few entries visible below it (see listScrollOff).
	listOffset int

	viewport viewport.Model
	width    int
	height   int

	// listWidth is the left pane's total width, border included. lipgloss Width
	// sizes the block *inside* the frame, so the drawable content is listWidth-2.
	listWidth int
}

func newModel(lib *core.Library, lang string) model {
	ti := textinput.New()
	ti.Placeholder = "search: technique, tag, tool, ATT&CK id, category:…"
	ti.Prompt = "› "
	ti.CharLimit = 160
	ti.Focus()

	m := model{
		lib:      lib,
		lang:     core.NormalizeLang(lang),
		input:    ti,
		viewport: viewport.New(60, 20),
	}
	m.matches = lib.Search("", 0)
	m.syncDetail()
	return m
}

func (m model) Init() tea.Cmd { return textinput.Blink }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		// Re-render: the body was laid out for the previous width.
		m.syncDetail()
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "esc":
			// Esc clears the search first, then exits. `q` is deliberately not a
			// quit key: it has to be typeable, since tools like qsfuzz exist and
			// searches like "sqli" contain it.
			if m.input.Value() != "" {
				m.input.SetValue("")
				m.refilter()
				return m, nil
			}
			return m, tea.Quit

		case "tab":
			if m.lang == core.LangZH {
				m.lang = core.LangEN
			} else {
				m.lang = core.LangZH
			}
			m.syncDetail()
			return m, nil

		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
				m.syncDetail()
			}
			return m, nil

		case "down", "ctrl+n":
			if m.cursor < len(m.matches)-1 {
				m.cursor++
				m.syncDetail()
			}
			return m, nil

		case "home":
			m.cursor = 0
			m.syncDetail()
			return m, nil

		case "end":
			if len(m.matches) > 0 {
				m.cursor = len(m.matches) - 1
				m.syncDetail()
			}
			return m, nil

		case "pgup", "pgdown", "ctrl+u", "ctrl+d":
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != m.query {
		m.refilter()
	}
	return m, cmd
}

// rowsPerItem is how many terminal rows one list entry occupies: its id and its
// title. The list window and the mouse handler must agree on this.
const rowsPerItem = 2

// listScrollOff is how many entries stay visible above and below the cursor.
//
// Without it the window only scrolls once the cursor already sits on the last
// visible row, so the list reads as if it ended at the highlighted entry and
// there is no hint that more follow. Keeping context on both sides means the
// next entries are on screen before they are needed.
const listScrollOff = 3

// mouseScrollLines is how far one wheel notch scrolls the detail pane.
const mouseScrollLines = 3

// listWindow returns the index of the first visible entry and how many fit.
func (m model) listWindow() (start, visible int) {
	visible = m.viewport.Height / rowsPerItem
	if visible < 1 {
		visible = 1
	}
	start = m.listOffset
	if maxStart := len(m.matches) - visible; start > maxStart {
		start = maxStart
	}
	if start < 0 {
		start = 0
	}
	return start, visible
}

// clampListOffset moves the window so the cursor sits at least listScrollOff
// entries away from both edges, as far as the list length allows. Moving inside
// that band does not scroll, which keeps the highlighted row where the eye
// expects it instead of dragging it to the edge first.
func (m *model) clampListOffset() {
	visible := m.viewport.Height / rowsPerItem
	if visible < 1 {
		visible = 1
	}

	off := listScrollOff
	if off*2 >= visible {
		// A short window has no room for context on both sides; keep what fits.
		off = (visible - 1) / 2
	}

	if m.cursor > m.listOffset+visible-1-off {
		m.listOffset = m.cursor - visible + 1 + off
	}
	if m.cursor < m.listOffset+off {
		m.listOffset = m.cursor - off
	}

	if maxStart := len(m.matches) - visible; m.listOffset > maxStart {
		m.listOffset = maxStart
	}
	if m.listOffset < 0 {
		m.listOffset = 0
	}
}

// handleMouse routes wheel and click events to whichever pane the pointer is
// over: scrolling the list moves the selection (the detail follows it), scrolling
// the detail moves the technique.
func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	inList := msg.X < m.listWidth

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if inList {
			if m.cursor > 0 {
				m.cursor--
				m.syncDetail()
			}
			return m, nil
		}
		m.viewport.LineUp(mouseScrollLines)
		return m, nil

	case tea.MouseButtonWheelDown:
		if inList {
			if m.cursor < len(m.matches)-1 {
				m.cursor++
				m.syncDetail()
			}
			return m, nil
		}
		m.viewport.LineDown(mouseScrollLines)
		return m, nil
	}

	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft && inList {
		if i, ok := m.rowAt(msg.Y); ok {
			m.cursor = i
			m.syncDetail()
		}
	}
	return m, nil
}

// rowAt maps a terminal row to a list index. The rows above the list content are
// the header, the search box and the pane's top border.
func (m model) rowAt(y int) (int, bool) {
	const listTop = 3
	row := y - listTop
	if row < 0 {
		return 0, false
	}

	start, visible := m.listWindow()
	i := start + row/rowsPerItem
	if i < start || i >= start+visible || i >= len(m.matches) {
		return 0, false
	}
	return i, true
}

func (m model) View() string {
	if m.width == 0 {
		return "loading…"
	}

	header := lipgloss.JoinHorizontal(lipgloss.Top,
		headerStyle.Render("HACK4ALL"),
		dimStyle.Render(fmt.Sprintf("  %d techniques", m.lib.Len())),
		langBadgeStyle.Render(langName(m.lang)),
	)

	search := searchStyle.Width(m.width).Render(m.input.View())

	// lipgloss sizes the block *inside* the border: Width(n) yields n columns of
	// content (plus padding), and the frame is added on top. So the panes are one
	// frame smaller than the space they occupy. Getting this wrong makes the view
	// wider than the terminal, lipgloss wraps the over-wide lines, the view grows
	// taller than the screen, and bubbletea scrolls the top of it away — which is
	// exactly how the list pane silently disappears.
	left := listPaneStyle.
		Width(m.listWidth - 2).
		Height(m.viewport.Height).
		Render(m.renderList())

	right := detailPaneStyle.
		Width(m.viewport.Width).
		Height(m.viewport.Height).
		Render(m.viewport.View())

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)

	footer := footerStyle.Width(m.width).Render(fmt.Sprintf(
		"%d/%d   ↑↓ or wheel   PgUp/PgDn scroll   Tab language (%s)   Esc clear / exit   Ctrl+C exit",
		min(m.cursor+1, len(m.matches)), len(m.matches), langName(m.lang)))

	// No background colour anywhere on purpose: the terminal's own transparency
	// (a wallpaper, acrylic, a themed profile) is the user's choice to make. The
	// full-screen fill only pads the block so the layout cannot shift; the blank
	// cells stay transparent.
	return screenStyle.Width(m.width).Height(m.height).Render(
		lipgloss.JoinVertical(lipgloss.Left, header, search, body, footer))
}

// =============================================================================
// Layout and content
// =============================================================================

func (m *model) layout() {
	// Total width: list block + 1 gap + detail block.
	listOuter := m.width * 2 / 5
	if listOuter < 36 {
		listOuter = 36
	}
	if listOuter > 62 {
		listOuter = 62
	}
	if listOuter > m.width-34 {
		listOuter = m.width - 34
	}
	if listOuter < 22 {
		listOuter = 22
	}
	m.listWidth = listOuter

	// The viewport width is the detail pane's drawable area, borders excluded.
	detailOuter := m.width - listOuter - 1
	m.viewport.Width = detailOuter - 2
	if m.viewport.Width < 20 {
		m.viewport.Width = 20
	}

	// Rows: header(1) + search(1) + footer(1) + pane borders(2) + 1 slack.
	m.viewport.Height = m.height - 6
	if m.viewport.Height < 3 {
		m.viewport.Height = 3
	}

	m.input.Width = m.width - 6
}

func (m *model) refilter() {
	m.query = m.input.Value()
	m.matches = m.lib.Search(m.query, 0)
	m.cursor = 0
	m.syncDetail()
}

func (m *model) syncDetail() {
	if len(m.matches) == 0 {
		m.listOffset = 0
		m.viewport.SetContent(dimStyle.Render("no technique matches this search"))
		return
	}
	if m.cursor >= len(m.matches) {
		m.cursor = len(m.matches) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.clampListOffset()
	m.viewport.SetContent(m.detailText())
	m.viewport.GotoTop()
}

// detailText assembles the right-hand pane for the selected technique: a styled
// header, then the body rendered from Markdown.
//
// The non-interactive CLI keeps printing Technique.Plain (raw Markdown) on
// purpose — it has no colours to render into, and a script or agent reading
// stdout is better served by the original text than by escape sequences.
func (m *model) detailText() string {
	t := m.matches[m.cursor].Technique
	width := m.viewport.Width
	if width < 16 {
		width = 16
	}

	title, _ := t.Title.Get(m.lang)
	summary, _ := t.Summary.Get(m.lang)
	body, _ := t.Body.Get(m.lang)

	var b []string

	for _, l := range wrapLines(title, width) {
		b = append(b, titleStyle.Render(l))
	}

	meta := make([]string, 0, 3)
	if c := t.CategoryPath(); c != "" {
		meta = append(meta, c)
	}
	if t.Difficulty != "" {
		meta = append(meta, t.Difficulty)
	}
	if t.Updated != "" {
		meta = append(meta, t.Updated)
	}
	if len(meta) > 0 {
		b = append(b, metaStyle.Render(strings.Join(meta, "  ·  ")))
	}
	b = append(b, "")

	b = append(b, labelRow("attck", strings.Join(t.ATTACK, " "), width)...)
	b = append(b, labelRow("tags", strings.Join(t.Tags, " · "), width)...)
	b = append(b, labelRow("tools", strings.Join(t.Tools, " · "), width)...)

	if summary != "" {
		b = append(b, "")
		for _, l := range wrapLines(summary, width-2) {
			b = append(b, summaryStyle.Render(l))
		}
	}

	b = append(b, "")
	b = append(b, strings.Split(renderMarkdown(body, width), "\n")...)

	return strings.Join(b, "\n")
}

// labelRow renders one "label  value" line, wrapping the value under itself.
func labelRow(label, value string, width int) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	const labelWidth = 6
	avail := width - labelWidth
	if avail < 8 {
		avail = 8
	}

	wrapped := wrapLines(value, avail)
	out := make([]string, 0, len(wrapped))
	for i, l := range wrapped {
		if i == 0 {
			out = append(out, labelStyle.Render(fmt.Sprintf("%-*s", labelWidth, label))+valueStyle.Render(l))
		} else {
			out = append(out, strings.Repeat(" ", labelWidth)+valueStyle.Render(l))
		}
	}
	return out
}

func (m model) renderList() string {
	start, visible := m.listWindow()

	inner := m.listWidth - 2 // the pane's drawable width
	textWidth := inner - 2   // minus the item padding
	var b strings.Builder

	for i := start; i < len(m.matches) && i < start+visible; i++ {
		t := m.matches[i].Technique
		idText := truncate(t.ID, textWidth)
		titleText := truncate(t.TitleFor(m.lang), textWidth)

		if i == m.cursor {
			b.WriteString(selectedStyle.Width(inner).Render(idText + "\n" + titleText))
		} else {
			b.WriteString(itemStyle.Width(inner).Render(itemIDStyle.Render(idText) + "\n" + titleText))
		}
		b.WriteString("\n")
	}
	if len(m.matches) == 0 {
		b.WriteString(dimStyle.Render("(nothing)"))
	}

	return b.String()
}

// =============================================================================
// Text wrapping
// =============================================================================

// wrapLines wraps text and returns the individual lines.
func wrapLines(s string, width int) []string {
	return strings.Split(wrapText(s, width), "\n")
}

// wrapText wraps text to width terminal cells.
//
// The viewport does not wrap on its own: it clips. Unwrapped, a paragraph is cut
// off mid-word and the rest of the line is unreachable. Horizontal rules are left
// alone; breaking a line of ─── in two helps nobody.
func wrapText(s string, width int) string {
	if width <= 0 {
		return s
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if isRule(strings.TrimSpace(line)) {
			out = append(out, line)
			continue
		}
		out = append(out, wrapLine(line, width)...)
	}
	return strings.Join(out, "\n")
}

// wrapLine breaks one line on spaces when it can and between characters when it
// cannot — Chinese has no spaces, so hard breaking is the only option there.
func wrapLine(line string, width int) []string {
	if width < 8 {
		width = 8
	}
	if runewidth.StringWidth(line) <= width {
		return []string{line}
	}

	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	body := strings.TrimLeft(line, " \t")
	avail := width - runewidth.StringWidth(indent)
	if avail < 8 {
		indent, avail = "", width
	}

	var out []string
	var cur strings.Builder
	curWidth, lastSpace := 0, -1

	flush := func() {
		out = append(out, indent+strings.TrimRight(cur.String(), " "))
		cur.Reset()
		curWidth, lastSpace = 0, -1
	}

	for _, r := range body {
		rw := runewidth.RuneWidth(r)
		if curWidth+rw > avail && curWidth > 0 {
			if lastSpace >= 0 {
				// Break at the last space so English words survive intact.
				s := cur.String()
				head := strings.TrimRight(s[:lastSpace], " ")
				tail := strings.TrimLeft(s[lastSpace+1:], " ")
				out = append(out, indent+head)
				cur.Reset()
				cur.WriteString(tail)
				curWidth, lastSpace = runewidth.StringWidth(tail), -1
			} else {
				flush()
			}
		}
		if r == ' ' {
			lastSpace = cur.Len()
		}
		cur.WriteRune(r)
		curWidth += rw
	}
	if strings.TrimSpace(cur.String()) != "" {
		flush()
	}

	return out
}

// isRule reports whether a line is a horizontal rule, e.g. ───── or -----.
func isRule(s string) bool {
	rs := []rune(s)
	if len(rs) < 4 {
		return false
	}
	for _, r := range rs {
		if r != rs[0] {
			return false
		}
	}
	return strings.ContainsRune("-=─═_*", rs[0])
}

// truncate cuts a string to width cells, counting CJK characters as two so a
// Chinese title cannot push the layout sideways.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= width {
		return s
	}
	return runewidth.Truncate(s, width, "…")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func langName(lang string) string {
	if core.NormalizeLang(lang) == core.LangZH {
		return "中文"
	}
	return "EN"
}

// =============================================================================
// Styles
// =============================================================================

// Colours only, never backgrounds — except where a small patch of colour is the
// point (the selected row, inline code, a code block). Filling the screen with a
// background would hide whatever the user deliberately set their terminal to.
var (
	colFG   = lipgloss.Color("252")
	colDim  = lipgloss.Color("243")
	colAcc  = lipgloss.Color("212")
	colAcc2 = lipgloss.Color("86")
	colLine = lipgloss.Color("238")

	// Pads the block to the terminal so the layout cannot jitter. Blank cells
	// carry no colour, so a transparent terminal stays transparent.
	screenStyle = lipgloss.NewStyle()

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colAcc).
			Padding(0, 1)

	langBadgeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("232")).
			Background(colAcc2).
			Bold(true).
			Padding(0, 1)

	dimStyle    = lipgloss.NewStyle().Foreground(colDim)
	searchStyle = lipgloss.NewStyle().Padding(0, 1)

	listPaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colLine)

	detailPaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colLine).
			Foreground(colFG)

	itemStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(colFG)

	itemIDStyle = lipgloss.NewStyle().Foreground(colDim)

	selectedStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Bold(true).
			Foreground(lipgloss.Color("231")).
			Background(lipgloss.Color("61"))

	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231"))
	metaStyle    = lipgloss.NewStyle().Foreground(colDim)
	labelStyle   = lipgloss.NewStyle().Foreground(colAcc2)
	valueStyle   = lipgloss.NewStyle().Foreground(colFG)
	summaryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("223")).Italic(true)

	footerStyle = lipgloss.NewStyle().
			Foreground(colDim).
			Padding(0, 1)
)
