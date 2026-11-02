// Package locales embeds the interface translations: one <language>.json
// file per language, mapping the English text (the key) to its
// translation, or to plural forms ({"one": ..., "other": ...}).
package locales

import "embed"

// FS holds the message files at its root.
//
//go:embed *.json
var FS embed.FS
