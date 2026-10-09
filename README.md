# Hack4all

**A bilingual technique guide for penetration testing, red teaming and bug bounty hunting.**
**面向渗透测试 / 红队 / bug bounty 的双语技术点指南。**

[中文](README.zh.md) | English

One Go binary, one knowledge base, three ways in: an interactive terminal UI, a
local web page, and a non-interactive command line that scripts and AI agents
can call. Content is written in English first with Chinese alongside, so a
newcomer and a veteran can read the same page.

一个 Go 二进制、一份内容库、三种入口：终端界面、本地网页、以及给脚本和 AI 用的非交互命令行。内容英文优先、中文并存，新人和老手看的是同一页。

---

## Install

Download one self-contained binary from the
[releases page](https://github.com/guaidao2/hack4all/releases) — Linux, macOS
and Windows, amd64 and arm64 — and run it. The knowledge base is embedded and
the binaries are static (`CGO_ENABLED=0`), so there is nothing else to install.

```bash
# Linux / macOS
chmod +x hack4all_v1.5.1_linux_amd64
./hack4all_v1.5.1_linux_amd64 -x "kerberos"

# Windows (PowerShell)
.\hack4all_v1.5.1_windows_amd64.exe -x "kerberos"
```

Check a download against the checksums published alongside it:

```bash
sha256sum -c hack4all_1.0.0_checksums.txt         # Linux
shasum -a 256 -c hack4all_1.0.0_checksums.txt     # macOS
```

To build it yourself instead, see [Build](#build) below.

## Three front-ends, one library

| Entry point | Command | For |
|---|---|---|
| TUI | `hack4all` | Reading in a terminal: search left, technique right, `Tab` switches language |
| Web | `hack4all web` | Reading in a browser at `http://127.0.0.1:8080` |
| CLI | `hack4all -x "QUERY"` | One line in a shell, or a tool/agent pulling a technique |

```bash
# interactive
hack4all
hack4all --lang zh                 # start the TUI in Chinese

# local web UI (127.0.0.1 only, no telemetry, no backend)
hack4all web
hack4all web --addr 127.0.0.1:9000 --no-open

# non-interactive: one technique, rendered for a terminal
hack4all -x "kerberos"
hack4all -x "NTLM 中继" --lang zh

# non-interactive: structured, for scripts and AI agents
hack4all -x "cloud metadata" --json
hack4all -x "ssrf" --json --limit 3 | jq '.matches[].id'

# browse
hack4all list
hack4all list --category offensive/web

# narrow a search by field
hack4all -x "category:offensive/credential-access"
hack4all -x "attck:T1558"
hack4all -x "tool:hashcat platform:linux"

# see a technique's structure before reading all of it
hack4all -x "kerberos" --outline
hack4all -x "category:offensive" --outline --limit 0   # the whole guide's outline

# validate the knowledge base (this is what CI runs)
hack4all check
hack4all check --strict
```

Exit codes: `0` success, `1` real error (bad flags, unreadable content),
`2` the query ran but matched nothing — the same convention `grep` uses, so a
script can tell "nothing found" from "something broke".

### Query syntax

Free words are ANDed. Field prefixes narrow the search before ranking, which is
what makes `-x` useful to a script or an agent: it can ask a precise question
instead of hoping free text ranks well.

| Prefix | Matches |
|---|---|
| `category:offensive/web` | the category subtree |
| `tag:kerberos` | tag present |
| `tool:hashcat` | tool present |
| `attck:T1558` | ATT&CK id present |
| `platform:windows` | platform present |
| `difficulty:intermediate` | difficulty present |
| `id:kerberoasting` | exact technique id |

Values match as case-insensitive substrings, so `attck:T1558` finds
`T1558.003`. An unrecognised prefix such as `foo:bar` stays a free-text word: a
query is never silently swallowed by a field the tool does not know about.
Both languages are indexed at once, so a Chinese query finds an entry whose
English text is what contains the term, and the reverse.

### The web UI

`hack4all web` serves the same library as one page: the category tree, search,
and a detail pane with a table of contents built from the section headings, plus
a copy button on every code block. It binds to `127.0.0.1` by default, and there
is no backend, no telemetry, no CDN and no build step — the page works on a
machine with no internet at all.

### The TUI

The interactive view renders the Markdown rather than printing it: headings lose
their hashes, tables become aligned columns, code fences become indented blocks,
lists get bullets, and emphasis becomes colour. `Tab` switches the display
language without losing your place in the list.

Rendering is deliberately done by the project itself rather than by a general
Markdown library, for two measured reasons: the common terminal renderers do not
wrap Chinese (no spaces to break on, so paragraphs overflow and get clipped),
and they paint a document background that would defeat a transparent terminal.
`hack4all -x` still prints the raw Markdown, because escape sequences are useless
to a pipe.

---

## Content format

One technique = one Markdown file. Both languages live in the same file, split
by two comment markers. Keeping them together (rather than `en/` and `zh/`
trees) means a translation cannot silently drift away from the text it belongs
to.

```markdown
---
id: kerberoasting
title_en: Kerberoasting
title_zh: Kerberoasting：Kerberos 服务票据离线破解
summary_en: One-line English summary.
summary_zh: 一句话中文摘要。
tags: [active-directory, kerberos, windows]
tools: [impacket-GetUserSPNs, Rubeus, hashcat]
attck: [T1558.003]
platform: [windows, linux]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Why it works
...English body, with fenced code blocks and tables...

<!-- lang:zh -->
### 原理
...中文正文，与英文信息量对等...
```

| Frontmatter field | Required | Notes |
|---|---|---|
| `id` | no | Defaults to the filename. Must be unique; it is what `-x` matches exactly |
| `title_en` / `title_zh` | at least one | Shown in lists and detail views |
| `summary_en` / `summary_zh` | no | One or two lines, shown above the body |
| `tags` | no | Free-form; searched |
| `tools` | no | Tool names referenced by the technique |
| `attck` | no | ATT&CK technique ids, e.g. `T1558.003` |
| `platform` | no | `windows`, `linux`, `cloud`, … |
| `difficulty` | no | `beginner` / `intermediate` / `advanced` |
| `updated` | no | `YYYY-MM-DD` |

> **A note on YAML.** A bare `": "` inside a frontmatter value is read as a
> nested key and the whole file stops loading. Either rephrase it, or quote the
> value: `summary_en: "Careful here: this is fine."`. `hack4all check` points
> this out when it happens, because it is the easiest mistake to make by hand.

**Categories come from the directory, never from frontmatter.** One source of
truth, so the tree and the metadata cannot disagree:

```
content/topics/offensive/credential-access/kerberoasting.md
         └─────┘ └────────┘ └───────────────┘
          root      category          technique
```

Adding a technique is adding a file. Adding a category is adding a directory.
No Go code changes, no index to regenerate.

Draft files and directories starting with `_` or `.` are skipped by the loader.

---

## Project layout

```
cmd/hack4all/        CLI entry point: subcommand dispatch, flags, -x mode
internal/core/       the shared library: parse, index, search, JSON shape
internal/tui/        interactive terminal UI (bubbletea)
  markdown.go        terminal Markdown renderer: styling and wrapping together
internal/web/        HTTP server + JSON API + embedded single-page front-end
content/             the knowledge base, embedded into the binary
  embed.go           go:embed of topics/
  topics/            one Markdown file per technique
```

`internal/core` is the only place that knows what a technique *is*. Every
front-end reads from it and none of them keeps its own copy of the data — that
is the rule that keeps the three entries from drifting apart.

`hack4all -x --json` and the web API return the literally same
`core.SearchResponse` structure.

---

## Build

Requires Go 1.24+.

```bash
go build -o bin/hack4all ./cmd/hack4all
./bin/hack4all -x "kerberos"
```

Cross-compile a static binary (the knowledge base is embedded, so the single
file is all you need to ship):

```bash
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -o dist/hack4all-linux-amd64   ./cmd/hack4all
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/hack4all-windows-amd64.exe ./cmd/hack4all
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -o dist/hack4all-darwin-arm64  ./cmd/hack4all
```

### Previewing content without rebuilding

`--content DIR` reads techniques from a directory instead of the embedded copy,
so a contributor can edit Markdown and re-run immediately:

```bash
hack4all --content ./content/topics -x "kerberos"
hack4all web --content ./content/topics
```

The directory layout under `DIR` is the same as under `content/topics/`.

---

## Reading a note on distribution

Attack-technique Markdown trips antivirus and EDR content heuristics, and files
can be quarantined in transit. When this project starts shipping archives,
consider an encrypted zip (a publicly documented password) rather than a plain
tarball — the encryption is not access control, it is a way to stop scanners
from silently eating the payload. Something to decide at release time, not now.

---

## Status

Stable as of 1.0. The three front-ends work end to end, the TUI renders the
Markdown, the content format is frozen and `hack4all check` guards it. The
knowledge base covers 151 techniques across 26 categories (including the
beginner track), and is still
growing. If you are planning or finishing a red team engagement, start with the
tactical map — `hack4all -x id:red-team-tactical-map` — which is a coverage
checklist for the whole engagement rather than a single technique. The web UI
draws the same file as a page at `/map.html`: stages as a timeline, tickable
items, and a progress bar you can keep open while you work.

Tags may carry Chinese terms as well as English ones (`越权`, `未授权访问`), because
readers search in whichever language they think in, and a filename-derived id is
not always the word they will reach for.

## Scope and content policy

This guide is for authorised security work: penetration tests, red team
engagements, in-scope bug bounty programmes, CTFs, and the defensive engineering
that detects the same techniques. What it documents is the same material that
public references — MITRE ATT&CK, vendor research, conference talks — already
document.

Three editorial rules apply to every contribution:

- **Technique over weapon.** Explain how a class of attack works, what it looks
  like from the defender's side, and how it is detected and mitigated. Do not
  add ready-to-run payload chains. A proof that demonstrates access is in scope;
  anything whose only effect is destroying data or taking a service down is not.
- **Detection is not optional.** Every technique is expected to say what
  telemetry it produces. An entry without a detection section is incomplete.
- **English first, Chinese alongside.** Both languages, in that order, in one
  file.

Neither the authors nor the contributors accept responsibility for misuse. If
you are testing a system you do not have written permission to test, none of
this is for you.

## License

MIT — see [LICENSE](LICENSE).

Copyright (c) 2026 guaidao2 & coolmoon.
