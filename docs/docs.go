// Package docs carries embisto's documentation, so a site can render the pages of the version it builds against.
package docs

import "embed"

// Files are the Markdown pages and the images they show, at the paths the pages link to.
//
//go:embed *.md assets
var Files embed.FS
