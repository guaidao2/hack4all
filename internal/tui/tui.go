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

	"github.com/guaidao2/hack4all/internal/core"
)

// Run starts the interactive UI and blocks until the user quits.
func Run(lib *core.Library, lang string) error {
	if lib.Len() == 0 {
		return fmt.Errorf("the knowledge base is empty")
	}
	m := newModel(lib, lang)
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
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

	viewport  viewport.Model
	width     int
	height    int
	listWidth int
}

func newModel(lib *core.Library, lang string) model {
	ti := textinput.New()
	ti.Placeholder = "search: technique, tag, tool, ATT&CK id…"
	ti.Prompt = "› "
	ti.CharLimit = 120
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
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "esc":
			if m.input.Value() != "" {
				m.input.SetValue("")
				m.refilter()
				return m, nil
			}
			return m, tea.Quit

		case "q":
			// Only quit on q when it is not being typed into the search box.
			if m.input.Value() == "" {
				return m, tea.Quit
			}

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

func (m model) View() string {
	if m.width == 0 {
		return "loading…"
	}

	header := headerStyle.Render("HACK4ALL") +
		dimStyle.Render(fmt.Sprintf("  %d techniques  ·  %s", m.lib.Len(), langName(m.lang)))

	search := searchStyle.Width(m.width - 2).Render(m.input.View())

	left := m.renderList()
	right := paneStyle.
		Width(m.viewport.Width).
		Height(m.viewport.Height).
		Render(m.viewport.View())

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)

	footer := dimStyle.Render(
		fmt.Sprintf("  %d/%d  ·  ↑↓ move  ·  PgUp/PgDn scroll  ·  Tab %s  ·  Esc clear/quit  ·  q quit",
			min(m.cursor+1, len(m.matches)), len(m.matches), langName(m.lang)))

	return lipgloss.JoinVertical(lipgloss.Left, header, search, body, footer)
}

// =============================================================================
// Layout and content
// =============================================================================

func (m *model) layout() {
	listWidth := m.width * 2 / 5
	if listWidth < 30 {
		listWidth = 30
	}
	if listWidth > 58 {
		listWidth = 58
	}
	if listWidth > m.width-24 {
		listWidth = m.width - 24
	}
	m.listWidth = listWidth

	m.viewport.Width = m.width - listWidth - 4
	if m.viewport.Width < 20 {
		m.viewport.Width = 20
	}
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
		m.viewport.SetContent(dimStyle.Render("\n  no technique matches this search"))
		return
	}
	if m.cursor >= len(m.matches) {
		m.cursor = len(m.matches) - 1
	}
	m.viewport.SetContent(m.matches[m.cursor].Technique.Plain(m.lang))
	m.viewport.GotoTop()
}

func (m model) renderList() string {
	visible := m.viewport.Height
	if visible < 1 {
		visible = 1
	}

	start := 0
	if m.cursor >= visible {
		start = m.cursor - visible + 1
	}

	var b strings.Builder
	for i := start; i < len(m.matches) && i < start+visible; i++ {
		t := m.matches[i].Technique
		line := truncate(t.ID+"  ·  "+t.TitleFor(m.lang), m.listWidth-2)
		if i == m.cursor {
			b.WriteString(selectedStyle.Width(m.listWidth - 2).Render(line))
		} else {
			b.WriteString(itemStyle.Width(m.listWidth - 2).Render(line))
		}
		b.WriteString("\n")
	}
	if len(m.matches) == 0 {
		b.WriteString(dimStyle.Render("  (nothing)"))
	}

	return paneStyle.Width(m.listWidth).Height(m.viewport.Height).Render(b.String())
}

// =============================================================================
// Styles and small helpers
// =============================================================================

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Padding(0, 1)
	searchStyle   = lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder(), false, false, false, false)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	itemStyle     = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("252"))
	selectedStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57"))
	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1)
)

func langName(lang string) string {
	if core.NormalizeLang(lang) == core.LangZH {
		return "中文"
	}
	return "EN"
}

// truncate cuts a string to width terminal cells, counting CJK characters as
// two cells so Chinese titles do not break the layout.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		w := 1
		if r > 0x1100 && (r <= 0x115F || (r >= 0x2E80 && r <= 0xA4CF) ||
			(r >= 0xAC00 && r <= 0xD7A3) || (r >= 0xF900 && r <= 0xFAFF) ||
			(r >= 0xFE30 && r <= 0xFE6F) || (r >= 0xFF00 && r <= 0xFF60) ||
			(r >= 0xFFE0 && r <= 0xFFE6)) {
			w = 2
		}
		if used+w > width {
			break
		}
		b.WriteRune(r)
		used += w
	}
	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
