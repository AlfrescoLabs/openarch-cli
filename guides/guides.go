// Package guides holds short guides for AI agents. The MCP server serves
// them as resources, and the binary embeds them, so they match the release.
package guides

import _ "embed"

// HxQL is the guide to the query language of Hyland OpenArch.
//
//go:embed hxql.md
var HxQL string
