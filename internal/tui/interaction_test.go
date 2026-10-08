package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/guaidao2/hack4all/internal/core"
)

func testModel(t *testing.T) model {
	t.Helper()
	m := newModel(realLibrary(t), core.LangEN)
	m.width, m.height = 100, 30
	m.layout()
	m.syncDetail()
	return m
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, quit := cmd().(tea.QuitMsg)
	return quit
}

// `q` has to be typeable. Tools called qsfuzz exist, and plenty of searches
// ("sqli", "sqli-sql-injection") contain it — a quit-on-q binding makes those
// impossible to type.
func TestTypingQDoesNotQuit(t *testing.T) {
	m := testModel(t)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got := updated.(model)

	if got.input.Value() != "q" {
		t.Errorf("search box = %q, want %q", got.input.Value(), "q")
	}
	if isQuit(cmd) {
		t.Fatal("pressing q quit the program")
	}
}

func TestTypingAQueryWithQKeepsWorking(t *testing.T) {
	m := testModel(t)

	// q must filter, not quit, whatever else it does.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = updated.(model)

	if m.input.Value() != "q" {
		t.Errorf("search box = %q, want q", m.input.Value())
	}
	if isQuit(cmd) {
		t.Fatal("pressing q quit the program")
	}
	// Keep typing: the list must follow the search box, not freeze on q. A
	// nonsense query coming back empty is what proves the filter actually ran.
	for _, r := range "zzz" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(model)
	}
	if m.input.Value() != "qzzz" {
		t.Errorf("search box = %q, want qzzz", m.input.Value())
	}
	if len(m.matches) != 0 {
		t.Errorf("a nonsense query matched %d entries", len(m.matches))
	}
}

func TestEscClearsThenQuits(t *testing.T) {
	m := testModel(t)
	m.input.SetValue("kerberos")
	m.refilter()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(model)
	if got.input.Value() != "" {
		t.Errorf("the first esc should clear the search, got %q", got.input.Value())
	}
	if isQuit(cmd) {
		t.Error("the first esc should not quit")
	}

	_, cmd = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !isQuit(cmd) {
		t.Error("esc on an empty search should quit")
	}
}

func TestWheelOverTheListMovesTheSelection(t *testing.T) {
	m := testModel(t)
	if m.cursor != 0 {
		t.Fatalf("cursor starts at %d, want 0", m.cursor)
	}

	updated, _ := m.Update(tea.MouseMsg{X: 3, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	got := updated.(model)
	if got.cursor != 1 {
		t.Errorf("cursor = %d after one notch over the list, want 1", got.cursor)
	}

	updated, _ = got.Update(tea.MouseMsg{X: 3, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if got = updated.(model); got.cursor != 0 {
		t.Errorf("cursor = %d after scrolling back, want 0", got.cursor)
	}
}

func TestWheelOverTheDetailScrollsWithoutMovingTheSelection(t *testing.T) {
	m := testModel(t)
	before := m.viewport.YOffset

	updated, _ := m.Update(tea.MouseMsg{
		X: m.listWidth + 5, Y: 10, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown,
	})
	got := updated.(model)

	if got.cursor != m.cursor {
		t.Errorf("scrolling the detail changed the selection: %d -> %d", m.cursor, got.cursor)
	}
	if got.viewport.YOffset <= before {
		t.Errorf("detail did not scroll: offset %d -> %d", before, got.viewport.YOffset)
	}
}

func TestWheelOverTheDetailStopsAtTheBottom(t *testing.T) {
	m := testModel(t)

	for i := 0; i < 200; i++ {
		updated, _ := m.Update(tea.MouseMsg{
			X: m.listWidth + 5, Y: 10, Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown,
		})
		m = updated.(model)
	}
	if m.viewport.YOffset > m.viewport.TotalLineCount() {
		t.Errorf("scroll offset %d exceeds the %d lines of content",
			m.viewport.YOffset, m.viewport.TotalLineCount())
	}
}

func TestClickSelectsAListEntry(t *testing.T) {
	m := testModel(t)

	// Rows 3+4 are the first entry (id, title), rows 5+6 the second.
	updated, _ := m.Update(tea.MouseMsg{X: 3, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if got := updated.(model); got.cursor != 1 {
		t.Errorf("clicking the second entry selected %d, want 1", got.cursor)
	}
}

func TestClickOnTheHeaderOrDetailChangesNothing(t *testing.T) {
	for _, pos := range []struct {
		name string
		x, y int
	}{
		{"header", 3, 0},
		{"detail pane", 90, 10},
		{"below the list", 3, 29},
	} {
		m := testModel(t)
		updated, _ := m.Update(tea.MouseMsg{X: pos.x, Y: pos.y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
		if got := updated.(model); got.cursor != m.cursor {
			t.Errorf("clicking the %s changed the selection: %d -> %d", pos.name, m.cursor, got.cursor)
		}
	}
}
