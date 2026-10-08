package tui

import (
	"os"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/guaidao2/hack4all/internal/core"
)

// TestPreviewRender is a scratch view of what the detail pane produces. Run with
// -v to eyeball the layout; it asserts nothing.
func TestPreviewRender(t *testing.T) {
	lib, err := core.Load(os.DirFS("../../content/topics"), ".")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	tech, ok := lib.Get("kerberoasting")
	if !ok {
		t.Fatal("kerberoasting not found")
	}
	body, _ := tech.Body.Get(core.LangEN)
	t.Log("\n===== en =====\n" + ansi.Strip(renderMarkdown(body, 64)))

	zh, _ := tech.Body.Get(core.LangZH)
	t.Log("\n===== zh =====\n" + ansi.Strip(renderMarkdown(zh, 64)))
}
