// Package web serves the knowledge base over HTTP so it can be read in a
// browser, with a small JSON API underneath.
//
// The API deliberately returns core.SearchResponse — the exact structure
// `hack4all -x --json` prints. An agent or script that learns one interface has
// learned the other, and the web UI is just another consumer of the same core.
package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/guaidao2/hack4all/internal/core"
)

//go:embed static
var staticFS embed.FS

// Config configures the local server.
type Config struct {
	Library     *core.Library
	Addr        string
	DefaultLang string
	OpenBrowser bool
}

// Handler returns the whole surface: the API plus the embedded front-end.
func (s *server) Handler() http.Handler {
	mux := s.apiMux()
	if sub, err := fs.Sub(staticFS, "static"); err == nil {
		mux.Handle("/", revalidate(http.FileServer(http.FS(sub))))
	}
	return mux
}

// revalidate marks the front-end as always worth re-checking. The assets are
// embedded, so a browser holding yesterday's stylesheet has no way to notice a
// new binary; with this it asks, and the file server answers 304 when nothing
// changed.
func revalidate(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}

// Serve runs the web UI until the process is stopped.
func Serve(cfg Config) error {
	if cfg.Library == nil || cfg.Library.Len() == 0 {
		return fmt.Errorf("the knowledge base is empty")
	}

	s := &server{
		lib:         cfg.Library,
		defaultLang: core.NormalizeLang(cfg.DefaultLang),
	}

	mux := s.Handler()

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}

	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		host, port = ln.Addr().String(), ""
	}
	local := bindIsLocal(ln.Addr())

	// The browser always runs on this machine, so localhost is the right URL
	// even when the listener is shared with the network.
	url := "http://localhost:" + port

	// ASCII only in the banner and in the addresses below: a Chinese Windows
	// console decodes our UTF-8 output as GBK, and even an em dash comes out as
	// mojibake. The URL is what an operator pastes into a browser, so it has to
	// survive every console.
	fmt.Printf("Hack4all web UI  ->  http://localhost:%s\n", port)
	if local {
		fmt.Printf("                     (this machine only)\n")
	} else {
		fmt.Printf("                     http://%s:%s  (all interfaces)\n", host, port)
		for _, ip := range localIPv4s() {
			fmt.Printf("                     http://%s:%s\n", ip, port)
		}
	}
	fmt.Printf("%d techniques loaded. Press Ctrl+C to stop.\n", cfg.Library.Len())
	if cfg.OpenBrowser {
		go openBrowser(url)
	}

	srv := &http.Server{
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return srv.Serve(ln)
}

// =============================================================================
// Handlers
// =============================================================================

type server struct {
	lib         *core.Library
	defaultLang string
}

// apiMux registers the JSON API on its own, so tests can exercise it without the
// embedded front-end and so the front-end can be mounted separately.
func (s *server) apiMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/search", s.handleSearch)
	mux.HandleFunc("/api/technique", s.handleTechnique)
	mux.HandleFunc("/api/categories", s.handleCategories)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/tactical-map", s.handleTacticalMap)
	return mux
}

func (s *server) lang(r *http.Request) string {
	if v := r.URL.Query().Get("lang"); v != "" {
		return core.NormalizeLang(v)
	}
	return s.defaultLang
}

// GET /api/search?q=&lang=&limit=&category=
func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	lang := s.lang(r)
	limit := atoiDefault(r.URL.Query().Get("limit"), 0)
	category := r.URL.Query().Get("category")

	var resp core.SearchResponse
	if category == "" {
		resp = s.lib.SearchResponse(q, lang, limit)
	} else {
		// Category browsing and text search are two different intents; when both
		// are given, search inside the category.
		resp = core.SearchResponse{Query: q, Lang: lang}
		for _, m := range s.lib.Search(q, 0) {
			if inCategory(m.Technique, category) {
				resp.Matches = append(resp.Matches, m.Technique.View(lang, m.Score))
				if limit > 0 && len(resp.Matches) >= limit {
					break
				}
			}
		}
		resp.Count = len(resp.Matches)
	}

	// The list view does not need the full body; it makes the payload large and
	// the UI never shows it before a click.
	if r.URL.Query().Get("slim") != "0" {
		for i := range resp.Matches {
			resp.Matches[i].Body = ""
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// GET /api/technique?id=&lang=
func (s *server) handleTechnique(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	t, ok := s.lib.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown technique: " + id})
		return
	}

	lang := s.lang(r)
	v := t.View(lang, 0)
	// The UI asks for the outline so it can build a table of contents; a plain
	// fetch does not pay for it.
	if r.URL.Query().Get("outline") == "1" {
		v = t.ViewWithHeadings(lang, 0)
	}
	writeJSON(w, http.StatusOK, v)
}

// GET /api/categories
func (s *server) handleCategories(w http.ResponseWriter, r *http.Request) {
	type category struct {
		Path  string `json:"path"`
		Name  string `json:"name"`
		Depth int    `json:"depth"`
		Count int    `json:"count"`
	}
	paths := s.lib.Categories()
	out := make([]category, 0, len(paths))
	for _, p := range paths {
		out = append(out, category{
			Path:  p,
			Name:  lastSegment(p),
			Depth: strings.Count(p, "/"),
			Count: s.lib.CountInCategory(p),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": out})
}

// GET /api/stats
func (s *server) handleStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"techniques": s.lib.Len(),
		"categories": len(s.lib.Categories()),
		"languages":  []string{core.LangEN, core.LangZH},
		"errors":     len(s.lib.Errors()),
	})
}

// GET /api/tactical-map?id=&lang=
//
// The coverage map is written as Markdown so it reads in a terminal and is
// searchable like any other entry; it is parsed here so the same file can also
// be drawn. One source, two presentations — the picture cannot drift away from
// the text.
func (s *server) handleTacticalMap(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		id = core.DefaultMapID
	}
	t, ok := s.lib.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown technique: " + id})
		return
	}
	m := core.TacticalMapOf(t, s.lang(r))
	if m == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not a coverage map: " + id})
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// =============================================================================
// Helpers
// =============================================================================

// lastSegment returns the final path element: "offensive/web" -> "web".
func lastSegment(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func inCategory(t *core.Technique, category string) bool {
	category = strings.Trim(strings.TrimSpace(category), "/")
	if category == "" {
		return true
	}
	p := t.CategoryPath()
	return p == category || strings.HasPrefix(p, category+"/")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s", r.Method, r.URL.RequestURI())
		}
		next.ServeHTTP(w, r)
	})
}

// bindIsLocal reports whether a listener is reachable only from this machine,
// which decides whether Serve prints the addresses a colleague could use.
func bindIsLocal(addr net.Addr) bool {
	if addr == nil {
		return false
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// localIPv4s lists this machine's non-loopback IPv4 addresses, so a shared
// instance can print URLs that someone can actually paste to a colleague.
func localIPv4s() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				out = append(out, ip4.String())
			}
		}
	}
	return out
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("could not open a browser automatically: %v", err)
	}
}
