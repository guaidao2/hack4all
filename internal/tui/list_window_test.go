package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/guaidao2/hack4all/internal/core"
)

// The list has to show entries beyond the selection. Before this, the window
// only scrolled once the cursor was already on the last visible row, so the pane
// looked as if it ended at the highlighted entry and gave no hint that more
// followed — you had to keep pressing down to discover them.
func TestListKeepsContextAroundTheCursor(t *testing.T) {
	m := newModel(realLibrary(t), core.LangEN)
	m.width, m.height = 100, 30
	m.layout()
	m.syncDetail()

	_, visible := m.listWindow()
	if visible < 2*listScrollOff+1 {
		t.Skipf("a %d-entry window is too small to assert context", visible)
	}

	// Step into the middle of the list, where the window is genuinely scrolling.
	for i := 0; i < visible+5 && m.cursor < len(m.matches)-1; i++ {
		m.cursor++
		m.syncDetail()
	}

	start, _ := m.listWindow()
	if below := start + visible - 1 - m.cursor; below < listScrollOff {
		t.Errorf("%d entries visible below the cursor, want at least %d (cursor=%d start=%d visible=%d)",
			below, listScrollOff, m.cursor, start, visible)
	}
	if above := m.cursor - start; above < listScrollOff {
		t.Errorf("%d entries visible above the cursor, want at least %d", above, listScrollOff)
	}
}

// Walking to the end must stop scrolling at the last entry rather than leaving
// blank rows, and the selected entry must still be rendered.
func TestListWindowStopsAtTheEnd(t *testing.T) {
	m := newModel(realLibrary(t), core.LangEN)
	m.width, m.height = 100, 30
	m.layout()
	m.syncDetail()

	for m.cursor < len(m.matches)-1 {
		m.cursor++
		m.syncDetail()
	}

	start, visible := m.listWindow()
	if start+visible > len(m.matches) {
		t.Errorf("window covers %d..%d but there are only %d entries", start, start+visible, len(m.matches))
	}
	if id := m.matches[m.cursor].Technique.ID; !strings.Contains(ansi.Strip(m.renderList()), id) {
		t.Errorf("the selected last entry (%s) is not visible:\n%s", id, ansi.Strip(m.renderList()))
	}
}

// A single-entry list must not produce a negative offset or an empty window.
func TestListWindowHandlesShortLists(t *testing.T) {
	lib := realLibrary(t)
	m := newModel(lib, core.LangEN)
	m.width, m.height = 100, 30
	m.layout()
	m.matches = m.matches[:1]
	m.cursor = 0
	m.syncDetail()

	start, visible := m.listWindow()
	if start != 0 {
		t.Errorf("start = %d, want 0", start)
	}
	if visible < 1 {
		t.Errorf("visible = %d, want at least 1", visible)
	}
	if !strings.Contains(ansi.Strip(m.renderList()), m.matches[0].Technique.ID) {
		t.Errorf("the only entry is not rendered")
	}
}
