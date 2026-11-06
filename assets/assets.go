// Package assets embeds the static files shipped with the game mode.
package assets

import "embed"

//go:embed skin/*.png
// Skin holds the generator skin images.
var Skin embed.FS
