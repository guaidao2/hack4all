package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/guaidao2/hack4all/internal/core"
)

func realLibrary(t *testing.T) *core.Library {
	t.Helper()
	lib, err := core.Load(os.DirFS("../../content/topics"), ".")
	if err != nil {
		t.Fatalf("load content: %v", err)
	}
	if lib.Len() == 0 {
		t.Fatal("no techniques loaded")
	}
	return lib
}

func lines(s string) int { return strings.Count(s, "\n") + 1 }

func maxWidth(s string) int {
	w := 0
	for _, l := range strings.Split(ansi.Strip(s), "\n") {
		if x := ansi.StringWidth(l); x > w {
			w = x
		}
	}
	return w
}

// The list pane must actually contain the matching entries: it is the only way
// to navigate, so a blank pane means the TUI is unusable however good the detail
// pane looks.
func TestRenderListContainsEntries(t *testing.T) {
	m := newModel(realLibrary(t), core.LangEN)
	m.width, m.height = 100, 30
	m.layout()
	m.syncDetail()

	out := ansi.Strip(m.renderList())
	if strings.TrimSpace(out) == "" {
		t.Fatal("renderList produced nothing")
	}

	// Assert against whatever the library holds rather than naming entries: the
	// visible window depends on how many techniques exist and which ids sort
	// first, so a hardcoded id breaks every time content is added.
	if len(m.matches) == 0 {
		t.Fatal("no matches to render")
	}
	_, visible := m.listWindow()
	if visible > len(m.matches) {
		visible = len(m.matches)
	}
	for i := 0; i < visible; i++ {
		if id := m.matches[i].Technique.ID; !strings.Contains(out, id) {
			t.Errorf("visible entry %d (%s) is missing from the list:\n%s", i, id, out)
		}
	}
}

func TestLayoutKeepsBothPanesVisible(t *testing.T) {
	m := newModel(realLibrary(t), core.LangEN)
	m.width, m.height = 100, 30
	m.layout()

	if m.listWidth < 20 {
		t.Errorf("list pane collapsed to %d columns", m.listWidth)
	}
	if m.viewport.Width < 20 {
		t.Errorf("detail pane collapsed to %d columns", m.viewport.Width)
	}
	// list block + gap + detail block must fit the terminal exactly.
	if got := m.listWidth + 1 + m.viewport.Width + 2; got != m.width {
		t.Errorf("panes span %d columns, terminal is %d", got, m.width)
	}
}

// Nothing may exceed the terminal. An over-wide view is wrapped by lipgloss, the
// wrap makes the view taller than the screen, and bubbletea then scrolls its top
// away — which is how the list pane silently disappeared on a real terminal.
func TestViewFitsTheTerminal(t *testing.T) {
	m := newModel(realLibrary(t), core.LangEN)
	m.width, m.height = 100, 30
	m.layout()
	m.syncDetail()

	left := listPaneStyle.Width(m.listWidth - 2).Height(m.viewport.Height).Render(m.renderList())
	right := detailPaneStyle.Width(m.viewport.Width).Height(m.viewport.Height).Render(m.viewport.View())
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)

	header := lipgloss.JoinHorizontal(lipgloss.Top,
		headerStyle.Render("HACK4ALL"),
		dimStyle.Render(fmt.Sprintf("  %d techniques", m.lib.Len())),
		langBadgeStyle.Render(langName(m.lang)))
	search := searchStyle.Width(m.width).Render(m.input.View())
	footer := footerStyle.Width(m.width).Render("footer text")

	vertical := lipgloss.JoinVertical(lipgloss.Left, header, search, body, footer)

	for _, part := range []struct{ name, s string }{
		{"header", header}, {"search", search}, {"body", body}, {"footer", footer},
	} {
		if w := maxWidth(part.s); w > m.width {
			t.Errorf("%s is %d columns wide, terminal is %d", part.name, w, m.width)
		}
	}
	if got := lines(vertical); got > m.height {
		t.Errorf("view is %d lines, terminal is %d", got, m.height)
	}

	view := ansi.Strip(m.View())
	if got := lines(view); got > m.height {
		t.Errorf("rendered view has %d lines, terminal has %d", got, m.height)
	}
	if !strings.Contains(view, "HACK4ALL") {
		t.Error("header missing from the view")
	}
	if strings.Count(view, "╭") < 2 {
		t.Errorf("expected two rounded pane corners, view:\n%s", view)
	}
	// Same reasoning as the list test: assert the first entry rather than a fixed
	// id, which stops being visible as soon as enough content exists.
	if id := m.matches[0].Technique.ID; !strings.Contains(view, id) {
		t.Errorf("the first list entry (%s) is missing from the rendered view:\n%s", id, view)
	}
}
