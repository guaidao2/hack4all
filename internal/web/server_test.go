package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/guaidao2/hack4all/internal/core"
)

func testLib(t *testing.T) *core.Library {
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

func apiFor(t *testing.T, lib *core.Library) http.Handler {
	t.Helper()
	return (&server{lib: lib, defaultLang: core.LangEN}).apiMux()
}

func testAPI(t *testing.T) (http.Handler, *core.Library) {
	t.Helper()
	lib := testLib(t)
	return apiFor(t, lib), lib
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func decodeSearch(t *testing.T, rec *httptest.ResponseRecorder) core.SearchResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp core.SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	return resp
}

func decodeView(t *testing.T, rec *httptest.ResponseRecorder) core.View {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var v core.View
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode view: %v", err)
	}
	return v
}

// The API and `hack4all -x --json` must stay interchangeable: an agent that
// learns one shape has learned the other, and the two drifting apart silently
// would be worse than either being wrong.
func TestAPISearchReturnsTheSameShapeAsTheCLI(t *testing.T) {
	h, _ := testAPI(t)
	resp := decodeSearch(t, get(t, h, "/api/search?q=kerberos"))

	if resp.Query != "kerberos" || resp.Lang != core.LangEN {
		t.Fatalf("unexpected envelope: %+v", resp)
	}
	// Ranking matters more than the count: the technique that is *about*
	// Kerberos has to come first, even though others mention it in passing.
	if len(resp.Matches) == 0 || resp.Matches[0].ID != "kerberoasting" {
		t.Fatalf("first match should be kerberoasting, got %+v", ids(resp.Matches))
	}
	m := resp.Matches[0]
	if m.Title.EN == "" || m.Title.ZH == "" {
		t.Errorf("title should stay bilingual in a list response: %+v", m.Title)
	}
	// Lists are slim: bodies are fetched per technique, otherwise one page would
	// ship the whole knowledge base.
	if m.Body != "" {
		t.Errorf("list response carries a %d character body", len(m.Body))
	}
}

func TestAPISearchCanIncludeBodies(t *testing.T) {
	h, _ := testAPI(t)
	resp := decodeSearch(t, get(t, h, "/api/search?q=kerberos&slim=0"))

	if len(resp.Matches) == 0 {
		t.Fatal("no matches")
	}
	if n := len(resp.Matches[0].Body); n < 500 {
		t.Errorf("body is only %d characters, expected the full technique", n)
	}
}

func TestAPISearchFindsChineseText(t *testing.T) {
	h, _ := testAPI(t)
	resp := decodeSearch(t, get(t, h, "/api/search?q=%E6%8F%90%E6%9D%83&lang=zh"))

	if len(resp.Matches) == 0 {
		t.Fatal("a Chinese query should match Chinese content")
	}
	if resp.Matches[0].ID != "linux-privilege-escalation" {
		t.Errorf("first match = %q, want linux-privilege-escalation", resp.Matches[0].ID)
	}
}

func TestAPISearchHonoursFieldQueries(t *testing.T) {
	h, _ := testAPI(t)
	resp := decodeSearch(t, get(t, h, "/api/search?q=id%3Akerberoasting"))

	if resp.Count != 1 || resp.Matches[0].ID != "kerberoasting" {
		t.Fatalf("id: query returned %+v", ids(resp.Matches))
	}
}

func TestAPISearchHonoursTheCategoryFilter(t *testing.T) {
	h, lib := testAPI(t)
	resp := decodeSearch(t, get(t, h, "/api/search?category=offensive/privilege-escalation"))

	want := lib.CountInCategory("offensive/privilege-escalation")
	if resp.Count != want || want == 0 {
		t.Fatalf("category filter returned %d matches, want %d", resp.Count, want)
	}
	for _, m := range resp.Matches {
		if got := strings.Join(m.Category, "/"); !strings.HasPrefix(got, "offensive/privilege-escalation") {
			t.Errorf("match %q is outside the requested category: %s", m.ID, got)
		}
	}
}

func TestAPISearchHonoursLimit(t *testing.T) {
	h, _ := testAPI(t)
	if got := decodeSearch(t, get(t, h, "/api/search?limit=2")).Count; got != 2 {
		t.Errorf("limit=2 returned %d matches", got)
	}
}

func TestAPITechniqueServesTheRequestedLanguage(t *testing.T) {
	h, _ := testAPI(t)

	en := decodeView(t, get(t, h, "/api/technique?id=kerberoasting&lang=en"))
	if en.BodyLang != core.LangEN || !strings.Contains(en.Body, "Kerberos service ticket") {
		t.Errorf("english request returned lang=%q", en.BodyLang)
	}

	zh := decodeView(t, get(t, h, "/api/technique?id=kerberoasting&lang=zh"))
	if zh.BodyLang != core.LangZH || !strings.Contains(zh.Body, "服务票据") {
		t.Errorf("chinese request returned lang=%q", zh.BodyLang)
	}
}

func TestAPITechniqueFallsBackWhenATranslationIsMissing(t *testing.T) {
	// Half-translated content is legitimate; the reader should still get text
	// rather than an empty pane, and the response says which language it served.
	lib, err := core.Load(fstest.MapFS{
		"topics/web/only-en.md": &fstest.MapFile{Data: []byte(
			"---\nid: only-en\ntitle_en: Only English\ntitle_zh: 只有英文\n---\n\n<!-- lang:en -->\nenglish body only\n")},
	}, "topics")
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}

	v := decodeView(t, get(t, apiFor(t, lib), "/api/technique?id=only-en&lang=zh"))
	if v.BodyLang != core.LangEN || !strings.Contains(v.Body, "english body only") {
		t.Errorf("expected a fallback to English, got lang=%q body=%q", v.BodyLang, v.Body)
	}
}

func TestAPIUnknownTechniqueIsNotFound(t *testing.T) {
	h, _ := testAPI(t)
	rec := get(t, h, "/api/technique?id=does-not-exist")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "does-not-exist") {
		t.Errorf("404 body should name the missing id: %s", rec.Body.String())
	}
}

func TestAPICategoriesCountsSubtrees(t *testing.T) {
	h, lib := testAPI(t)
	rec := get(t, h, "/api/categories")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}

	var out struct {
		Categories []struct {
			Path  string `json:"path"`
			Name  string `json:"name"`
			Depth int    `json:"depth"`
			Count int    `json:"count"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Categories) == 0 {
		t.Fatal("no categories returned")
	}

	byPath := map[string]int{}
	for _, c := range out.Categories {
		byPath[c.Path] = c.Count
		if c.Name == "" || strings.Contains(c.Name, "/") {
			t.Errorf("category %q should carry its leaf name, got %q", c.Path, c.Name)
		}
	}

	// A parent counts everything beneath it — that is the number the sidebar shows.
	// Asserted against the library rather than a literal, so adding content does
	// not break the test.
	for _, path := range []string{"offensive", "offensive/credential-access", "offensive/web"} {
		if want := lib.CountInCategory(path); byPath[path] != want {
			t.Errorf("category %q counts %d techniques, want %d", path, byPath[path], want)
		}
	}
}

func TestAPIStatsReportsTheLibrary(t *testing.T) {
	h, lib := testAPI(t)
	rec := get(t, h, "/api/stats")

	var out struct {
		Techniques int      `json:"techniques"`
		Categories int      `json:"categories"`
		Languages  []string `json:"languages"`
		Errors     int      `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Techniques != lib.Len() {
		t.Errorf("techniques = %d, library has %d", out.Techniques, lib.Len())
	}
	if out.Categories != len(lib.Categories()) {
		t.Errorf("categories = %d, library has %d", out.Categories, len(lib.Categories()))
	}
	if out.Errors != 0 {
		t.Errorf("errors = %d, want 0 for valid content", out.Errors)
	}
	if len(out.Languages) != 2 {
		t.Errorf("languages = %v, want both", out.Languages)
	}
}

func TestAPIResponsesDeclareUTF8(t *testing.T) {
	h, _ := testAPI(t)
	ct := get(t, h, "/api/stats").Header().Get("Content-Type")

	if !strings.Contains(ct, "application/json") || !strings.Contains(ct, "utf-8") {
		t.Errorf("Content-Type = %q, want JSON with utf-8 for the Chinese content", ct)
	}
}

func ids(matches []core.View) []string {
	out := make([]string, len(matches))
	for i, m := range matches {
		out[i] = m.ID
	}
	return out
}
