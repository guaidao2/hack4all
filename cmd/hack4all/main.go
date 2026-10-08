// Command hack4all is a bilingual knowledge base for penetration testing, red
// teaming and bug bounty work.
//
// One binary, three front-ends over one core content library:
//
//	hack4all                     interactive TUI
//	hack4all web                 local web UI
//	hack4all -x "kerberos"       non-interactive query (add --json for machines)
//
// The non-interactive mode exists so a single shell line — or an AI agent — can
// pull a technique without a human in the loop. Its JSON shape is exactly the
// one the web API serves, so both consumers learn the same structure once.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/guaidao2/hack4all/content"
	"github.com/guaidao2/hack4all/internal/core"
	"github.com/guaidao2/hack4all/internal/tui"
	"github.com/guaidao2/hack4all/internal/web"
	"github.com/mattn/go-isatty"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "0.1.0"

// exitNoMatch is returned when a query is valid but finds nothing. It is
// distinct from 1 (a real error) so scripts and agents can tell the two apart —
// the same convention grep uses.
const exitNoMatch = 2

func main() {
	err := run(os.Args[1:])
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return
	}
	// The content check prints its own detailed report; do not repeat it.
	if errors.Is(err, errCheckFailed) {
		os.Exit(1)
	}
	// Always explain ourselves on stderr, even for a no-match exit: a silent
	// non-zero exit is useless to a human and ambiguous to a script.
	fmt.Fprintf(os.Stderr, "hack4all: %v\n", err)
	if errors.As(err, &noMatchError{}) {
		os.Exit(exitNoMatch)
	}
	os.Exit(1)
}

// errCheckFailed means `hack4all check` found problems; the report is already on
// stdout, so main only has to set the exit status.
var errCheckFailed = errors.New("content check failed")

// noMatchError marks "the query ran fine, there is simply nothing to show".
type noMatchError struct{ query string }

func (e noMatchError) Error() string {
	return fmt.Sprintf("no technique matches %q", e.query)
}

func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "tui":
			return runTUI(args[1:])
		case "web", "serve":
			return runWeb(args[1:])
		case "query", "q", "search", "s":
			return runQuery(args[1:])
		case "list", "ls":
			return runList(args[1:])
		case "check":
			return runCheck(args[1:])
		case "version", "-v", "--version":
			fmt.Printf("hack4all %s\n", version)
			return nil
		case "help", "-h", "--help":
			return runHelp(args[1:])
		}
	}

	// A leading dash means flags, not a subcommand: `hack4all -x nmap --json`.
	if len(args) > 0 && strings.HasPrefix(args[0], "-") {
		return runQuery(args)
	}
	if len(args) == 0 {
		return runTUI(nil)
	}
	return fmt.Errorf("unknown command %q (try: hack4all help)", args[0])
}

// =============================================================================
// Non-interactive query
// =============================================================================

func runQuery(args []string) error {
	fs := newFlagSet("query")
	x := fs.String("x", "", "search terms, or an exact technique id")
	query := fs.String("query", "", "alias of -x")
	lang := fs.String("lang", core.DefaultLang, "content language: en or zh")
	asJSON := fs.Bool("json", false, "machine-readable output, same shape as the web API")
	outline := fs.Bool("outline", false, "list section headings instead of the full text")
	limit := fs.Int("limit", 1, "maximum matches (0 = all)")
	contentDir := fs.String("content", "", "load techniques from this directory instead of the embedded library")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Go's flag package stops parsing at the first positional argument, so
	// anything written after one is dropped — including a -lang that was placed
	// after a stray argument. Silently ignoring the flag someone just typed is
	// exactly the failure this project tries not to have, so say it out loud.
	if rest := fs.Args(); len(rest) > 0 && (*x != "" || *query != "") {
		fmt.Fprintf(os.Stderr, "hack4all: warning: ignoring extra argument(s): %s\n", strings.Join(rest, " "))
		if looksLikeFlag(rest) {
			fmt.Fprintln(os.Stderr, `hack4all: hint: flags must come before the query — try: hack4all -lang zh -x "QUERY"`)
		}
	}

	q := firstNonEmpty(*x, *query, strings.Join(fs.Args(), " "))
	if strings.TrimSpace(q) == "" {
		fs.Usage()
		return errors.New("-x requires a query, e.g. hack4all -x \"ntlm relay\"")
	}

	langCode := core.NormalizeLang(*lang)
	lib, err := openLibrary(*contentDir)
	if err != nil {
		return err
	}
	warnLoadErrors(lib)

	if *outline {
		matches := lib.Search(q, *limit)
		if len(matches) == 0 {
			return noMatchError{query: q}
		}
		if *asJSON {
			resp := core.SearchResponse{Query: q, Lang: langCode}
			for _, m := range matches {
				resp.Matches = append(resp.Matches, m.Technique.ViewWithHeadings(langCode, m.Score))
			}
			resp.Count = len(resp.Matches)
			return writeJSON(resp)
		}
		printOutline(matches, langCode)
		return nil
	}

	if *asJSON {
		return writeJSON(lib.SearchResponse(q, langCode, *limit))
	}

	matches := lib.Search(q, *limit)
	if len(matches) == 0 {
		return noMatchError{query: q}
	}
	for i, m := range matches {
		if i > 0 {
			fmt.Println(strings.Repeat("─", 72))
		}
		fmt.Print(m.Technique.Plain(langCode))
	}
	return nil
}

// printOutline writes only the section structure, so a reader — or an agent —
// can see what a technique covers before deciding to pull the whole body.
func printOutline(matches []core.Match, lang string) {
	for i, m := range matches {
		if i > 0 {
			fmt.Println()
		}
		t := m.Technique
		meta := t.CategoryPath()
		if meta != "" {
			meta = "  (" + meta + ")"
		}
		fmt.Printf("%s — %s%s\n", t.ID, t.TitleFor(lang), meta)
		for _, h := range t.Headings(lang) {
			fmt.Printf("%s%s\n", strings.Repeat("  ", max(0, h.Level-1)), h.Text)
		}
	}
}

// writeJSON encodes a machine-readable answer with the settings they all share:
// indented, and HTML escaping off so command examples stay copy-pasteable.
func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// =============================================================================
// List
// =============================================================================

func runList(args []string) error {
	fs := newFlagSet("list")
	lang := fs.String("lang", core.DefaultLang, "content language: en or zh")
	asJSON := fs.Bool("json", false, "machine-readable output")
	category := fs.String("category", "", "only techniques under this category prefix")
	contentDir := fs.String("content", "", "load techniques from this directory instead of the embedded library")
	if err := fs.Parse(args); err != nil {
		return err
	}

	langCode := core.NormalizeLang(*lang)
	lib, err := openLibrary(*contentDir)
	if err != nil {
		return err
	}
	warnLoadErrors(lib)

	if *asJSON {
		resp := core.SearchResponse{Query: "", Lang: langCode}
		for _, t := range lib.InCategory(*category) {
			resp.Matches = append(resp.Matches, t.View(langCode, 0))
		}
		resp.Count = len(resp.Matches)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(resp)
	}

	if lib.Len() == 0 {
		return noMatchError{query: "the knowledge base is empty"}
	}
	fmt.Println(core.RowHeader())
	for _, t := range lib.InCategory(*category) {
		fmt.Println(t.Row(langCode))
	}
	return nil
}

// =============================================================================
// Content check
// =============================================================================

// runCheck validates the knowledge base and reports what is broken or missing.
// It is meant to run before a commit and in CI: content that silently fails to
// load, or a translation that was never written, is invisible otherwise.
func runCheck(args []string) error {
	fs := newFlagSet("check")
	contentDir := fs.String("content", "", "check this directory instead of the embedded library")
	strict := fs.Bool("strict", false, "treat warnings as failures too")
	if err := fs.Parse(args); err != nil {
		return err
	}

	lib, err := openLibrary(*contentDir)
	if err != nil {
		return err
	}

	issues := lib.Validate()
	for _, is := range issues {
		where := is.Path
		if where == "" {
			where = "(library)"
		}
		fmt.Printf("%-7s %-48s %s\n", is.Level, where, is.Msg)
	}

	errCount, warnCount := core.CountIssues(issues)
	fmt.Printf("\n%d technique(s), %d error(s), %d warning(s)\n", lib.Len(), errCount, warnCount)

	if errCount > 0 || (*strict && warnCount > 0) {
		return errCheckFailed
	}
	return nil
}

// =============================================================================
// TUI and web
// =============================================================================

func runTUI(args []string) error {
	fs := newFlagSet("tui")
	lang := fs.String("lang", core.DefaultLang, "content language: en or zh")
	contentDir := fs.String("content", "", "load techniques from this directory instead of the embedded library")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Without a terminal the TUI would block forever waiting on input. That is
	// exactly what happens when a script or an agent runs `hack4all` with no
	// arguments, so fail fast with the non-interactive alternative instead.
	if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
		return fmt.Errorf("no interactive terminal on stdin; use `hack4all -x \"QUERY\"` or `hack4all list`, or `hack4all web`")
	}

	lib, err := openLibrary(*contentDir)
	if err != nil {
		return err
	}
	warnLoadErrors(lib)
	return tui.Run(lib, core.NormalizeLang(*lang))
}

func runWeb(args []string) error {
	fs := newFlagSet("web")
	host := fs.String("host", "127.0.0.1", "listen host; use 0.0.0.0 to share on the local network")
	port := fs.Int("port", 8080, "listen port; 0 picks a free one")
	addr := fs.String("addr", "", "full listen address host:port; overrides --host and --port")
	lang := fs.String("lang", core.DefaultLang, "default content language")
	contentDir := fs.String("content", "", "load techniques from this directory instead of the embedded library")
	noOpen := fs.Bool("no-open", false, "do not try to open a browser")
	if err := fs.Parse(args); err != nil {
		return err
	}

	listen := *addr
	if listen == "" {
		listen = net.JoinHostPort(*host, strconv.Itoa(*port))
	}

	lib, err := openLibrary(*contentDir)
	if err != nil {
		return err
	}
	warnLoadErrors(lib)
	return web.Serve(web.Config{
		Library:     lib,
		Addr:        listen,
		DefaultLang: core.NormalizeLang(*lang),
		OpenBrowser: !*noOpen,
	})
}

// =============================================================================
// Shared helpers
// =============================================================================

// openLibrary loads from a directory when one is given, otherwise from the
// copy embedded in the binary. Local content wins so that a contributor can
// preview edits without rebuilding.
func openLibrary(dir string) (*core.Library, error) {
	if dir != "" {
		info, err := os.Stat(dir)
		if err != nil {
			return nil, fmt.Errorf("content directory: %w", err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%s is not a directory", dir)
		}
		return core.Load(os.DirFS(filepath.ToSlash(dir)), ".")
	}
	return core.Load(content.FS, content.Root)
}

// warnLoadErrors reports unreadable or malformed files. A bad contribution must
// never be silent: content that silently disappears is worse than a loud warning.
func warnLoadErrors(lib *core.Library) {
	for _, err := range lib.Errors() {
		fmt.Fprintf(os.Stderr, "hack4all: warning: %v\n", err)
	}
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: hack4all %s [flags]\n\nflags:\n", name)
		fs.PrintDefaults()
	}
	return fs
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// looksLikeFlag reports whether any argument was probably meant to be a flag.
// That is the case in "hack4all -x QUERY extra -lang zh", where the language
// flag is swallowed because it came after a positional argument.
func looksLikeFlag(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			return true
		}
	}
	return false
}

// runHelp prints the help in the requested language.
func runHelp(args []string) error {
	fs := newFlagSet("help")
	lang := fs.String("lang", core.DefaultLang, "output language: en or zh")
	if err := fs.Parse(args); err != nil {
		return err
	}
	usage(os.Stdout, core.NormalizeLang(*lang))
	return nil
}

const usageZH = `Hack4all - 渗透测试 / 红队 / bug bounty 双语技术点指南

用法
  hack4all                        交互式终端界面
  hack4all web                    本地网页，默认 127.0.0.1:8080
  hack4all web --port 9000        8080 被占用时换个端口
  hack4all web --host 0.0.0.0     共享给局域网
  hack4all -x "查询"              输出一篇技术点
  hack4all -x "查询" --json       同上，机器可读（给脚本和 AI）
  hack4all list                   列出全部技术点
  hack4all list --category offensive/web
  hack4all check                  校验内容库（CI 会跑）
  hack4all version | help

查询语法
  空格分隔的词是「与」关系：      hack4all -x "ntlm relay"
  按字段限定：                    category:offensive/web  tag:kerberos  tool:hashcat
                                  attck:T1558  platform:windows  difficulty:intermediate
                                  id:kerberoasting
  组合使用：                      hack4all -x "category:offensive relay"

终端界面按键
  up / down、鼠标滚轮     在列表里移动；指针在右侧时滚轮滚动正文
  单击                    选中列表项
  PgUp / PgDn             滚动正文
  Tab                     切换语言（en / zh）
  Esc                     先清空搜索，再按一次退出
  Ctrl+C                  退出

参数
  -x, -query 字符串   搜索词，或精确的技术点 id
  -lang en|zh         内容语言（默认 en）
  -limit N            -x 模式最多返回几篇（默认 1，0 表示不限）
  -json               JSON 输出，与网页 API 结构一致
  -outline            只列章节标题，不输出全文
  -content 目录       从指定目录读内容，而不是用内置的

退出码
  0  成功
  1  出错（参数错误、内容不可读）
  2  查询执行了，但没有匹配

英文帮助：hack4all help（不带 -lang 时默认英文）
`

func usage(w io.Writer, lang string) {
	if core.NormalizeLang(lang) == core.LangZH {
		fmt.Fprint(w, usageZH)
		return
	}
	fmt.Fprint(w, usageEN)
}

// usageEN is the English help, deliberately ASCII-only: a Chinese Windows console
// decodes our UTF-8 output as GBK, and an arrow or a dash comes out as mojibake.
const usageEN = `Hack4all - a bilingual technique guide for penetration testing, red teaming and bug bounty hunting

USAGE
  hack4all                        interactive TUI
  hack4all web                    local web UI on 127.0.0.1:8080
  hack4all web --port 9000        a different port, when 8080 is taken
  hack4all web --host 0.0.0.0     share it with the local network
  hack4all -x "QUERY"             one technique, rendered for a terminal
  hack4all -x "QUERY" --json      the same result for scripts and AI agents
  hack4all list                   list every technique
  hack4all list --category offensive/web
  hack4all check                  validate the knowledge base (for CI)
  hack4all version | help

QUERY SYNTAX
  free words are ANDed:           hack4all -x "ntlm relay"
  narrow by field:                category:offensive/web  tag:kerberos  tool:hashcat
                                  attck:T1558  platform:windows  difficulty:intermediate
                                  id:kerberoasting
  combine them:                   hack4all -x "category:offensive relay"

TUI KEYS
  up / down, wheel      move in the list; over the detail pane the wheel scrolls
                        the technique instead
  click                 select a list entry
  PgUp / PgDn           scroll the technique
  Tab                   switch language (en / zh)
  Esc                   clear the search, then exit
  Ctrl+C                exit

FLAGS
  -x, -query STRING   search terms, or an exact technique id
  -lang en|zh         content language (default: en)
  -limit N            max matches in -x mode (default: 1, 0 = all)
  -json               JSON output, identical shape to the web API
  -outline            list section headings instead of the full text, so a
                      reader (or an agent) can see the structure first
  -content DIR        read techniques from DIR instead of the embedded library

EXIT CODES
  0  success
  1  error (bad flags, unreadable content)
  2  the query ran but matched nothing

Chinese help: hack4all help -lang zh
`
