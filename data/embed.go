// Package data exposes the distilled classification rules as an embed.FS for
// downstream consumers (notably brief). It has no dependencies beyond embed
// so importing it does not pull in the LLM client.
package data

import "embed"

//go:embed all:packages all:shapes
var FS embed.FS
