// Package content holds the Hack4all knowledge base: one Markdown file per
// technique, bilingual (English first, Chinese alongside).
//
// The directory doubles as a Go package so the whole knowledge base can be
// embedded into the binary. Everything lives under topics/, which is the single
// pattern go:embed points at: adding a new category or a new technique means
// adding files, never touching Go code.
//
// Why not embed "." ? The embed directive rejects "." and ".." as patterns, and
// it has no recursive wildcard; a fixed top-level directory is the supported
// way to embed a tree that keeps growing.
package content

import "embed"

// FS is the embedded knowledge base. It contains every file under topics/,
// including this package's own Go file, so loaders must filter by extension.
//
//go:embed all:topics
var FS embed.FS

// Root is the path inside FS under which the techniques live.
const Root = "topics"
