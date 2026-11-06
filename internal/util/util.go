// Package util contains the pure helpers shared across the game: colors, message formatting and
// coordinates.
package util

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/frogcreative/bedwars/internal/cfg"
)

// Chat color codes.
const (
	Black      = "§0"
	DarkBlue   = "§1"
	DarkGreen  = "§2"
	DarkAqua   = "§3"
	DarkRed    = "§4"
	DarkPurple = "§5"
	Gold       = "§6"
	Gray       = "§7"
	DarkGray   = "§8"
	Blue       = "§9"
	Green      = "§a"
	Aqua       = "§b"
	Red        = "§c"
	Pink       = "§d"
	Yellow     = "§e"
	White      = "§f"
)

// TeamColorOrder keeps the display order of the team colors.
var TeamColorOrder = []string{
	"BLACK", "DARK_BLUE", "DARK_GREEN", "DARK_AQUA", "DARK_RED", "DARK_PURPLE",
	"GOLD", "GRAY", "DARK_GRAY", "BLUE", "GREEN", "AQUA", "RED", "PINK", "YELLOW", "WHITE",
}

// TeamColor maps a color name to its chat color code.
var TeamColor = map[string]string{
	"BLACK": Black, "DARK_BLUE": DarkBlue, "DARK_GREEN": DarkGreen, "DARK_AQUA": DarkAqua,
	"DARK_RED": DarkRed, "DARK_PURPLE": DarkPurple, "GOLD": Gold, "GRAY": Gray,
	"DARK_GRAY": DarkGray, "BLUE": Blue, "GREEN": Green, "AQUA": Aqua, "RED": Red,
	"PINK": Pink, "YELLOW": Yellow, "WHITE": White,
}

// GetChatColor resolves a team or dye name to a chat color code, falling back to gray.
func GetChatColor(name string) string {
	n := strings.ReplaceAll(strings.ToUpper(name), " ", "_")
	if c, ok := TeamColor[n]; ok {
		return c
	}
	switch n {
	case "CYAN":
		return DarkAqua
	case "LIGHT_GRAY":
		return DarkGray
	case "ORANGE":
		return Gold
	}
	return "§7"
}

// EnName normalizes dye names to the team names used in arena files (Lime becomes Green, Light
// Blue becomes Aqua).
func EnName(wool string) string {
	switch wool {
	case "Lime":
		return "Green"
	case "Light Blue":
		return "Aqua"
	}
	return wool
}

// MaxInTeamToGroup converts the players-per-team value to its mode name (Solo, Doubles, 3v3v3v3,
// 4v4v4v4).
func MaxInTeamToGroup(max int) string {
	switch max {
	case 1:
		return "Solo"
	case 2:
		return "Doubles"
	case 3:
		return "3v3v3v3"
	case 4:
		return "4v4v4v4"
	}
	return "Custom"
}

// CalculateTime formats seconds as mm:ss.
func CalculateTime(sec int) string {
	if sec < 0 {
		sec = 0
	}
	// Minutes intentionally wrap after 59; this is a clock-style display, not an elapsed-hours value.
	return fmt.Sprintf("%02d:%02d", (sec/60)%60, sec%60)
}

// Ucfirst upper-cases the first byte of s.
func Ucfirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// Colorize converts "&" color markers (for example "&e") to section-sign color codes.
func Colorize(s string) string {
	const codes = "0123456789abcdefklmnor"
	b := []rune(s)
	for i := 0; i+1 < len(b); i++ {
		if b[i] == '&' && strings.ContainsRune(codes, b[i+1]) {
			b[i] = '§'
		}
	}
	return string(b)
}

// replace substitutes {name} placeholders using alternating name/value pairs.
func replace(s string, ph []any) string {
	for i := 0; i+1 < len(ph); i += 2 {
		s = strings.ReplaceAll(s, "{"+fmt.Sprint(ph[i])+"}", fmt.Sprint(ph[i+1]))
	}
	return s
}

// Msg returns the colorized message stored at key with placeholders substituted. Placeholders are
// passed as name/value pairs.
func Msg(key string, ph ...any) string {
	return replace(Colorize(cfg.String(key, "")), ph)
}

// MsgList is like Msg for a list of lines.
func MsgList(key string, ph ...any) []string {
	lines := cfg.Strings(key)
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, replace(Colorize(l), ph))
	}
	return out
}

// Vec is a 3D coordinate. Arena files store it as an "x:y:z" string.
type Vec struct{ X, Y, Z float64 }

// fmtFloat formats a float without trailing zeros.
func fmtFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// String returns the "x:y:z" arena-file representation.
func (v Vec) String() string { return fmtFloat(v.X) + ":" + fmtFloat(v.Y) + ":" + fmtFloat(v.Z) }

// Add returns the vector translated by (x, y, z).
func (v Vec) Add(x, y, z float64) Vec { return Vec{v.X + x, v.Y + y, v.Z + z} }

// Floor rounds every component down.
func (v Vec) Floor() Vec { return Vec{math.Floor(v.X), math.Floor(v.Y), math.Floor(v.Z)} }

// Ceil rounds every component up.
func (v Vec) Ceil() Vec { return Vec{math.Ceil(v.X), math.Ceil(v.Y), math.Ceil(v.Z)} }

// Distance returns the Euclidean distance to o.
func (v Vec) Distance(o Vec) float64 {
	dx, dy, dz := v.X-o.X, v.Y-o.Y, v.Z-o.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// ParseVec parses an "x:y:z" string.
func ParseVec(s string) (Vec, bool) {
	parts := strings.Split(s, ":")
	// Only the first three components are read; any trailing components are ignored.
	if len(parts) < 3 {
		return Vec{}, false
	}
	var f [3]float64
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64)
		if err != nil {
			return Vec{}, false
		}
		f[i] = v
	}
	return Vec{f[0], f[1], f[2]}, true
}

// MustVec parses an already validated arena coordinate and returns the zero vector on malformed
// input.
func MustVec(s string) Vec {
	v, _ := ParseVec(s)
	return v
}

// TeamRGB returns the leather armor color for a team name.
func TeamRGB(team string) (r, g, b uint8) {
	m := map[string][3]uint8{
		"white": {0xFF, 0xFF, 0xFF}, "orange": {0xFF, 0xA5, 0x00}, "magenta": {0xFF, 0x00, 0xFF},
		"aqua": {0x00, 0xFF, 0xFF}, "yellow": {0xFF, 0xFF, 0x00}, "green": {0x00, 0xFF, 0x00},
		"pink": {0xFF, 0x00, 0xFF}, "gray": {0x80, 0x80, 0x80}, "light gray": {0xC0, 0xC0, 0xC0},
		"cyan": {0x00, 0x00, 0x80}, "purple": {0x80, 0x00, 0x80}, "blue": {0x00, 0x00, 0xFF},
		"brown": {0xA5, 0x2A, 0x2A}, "red": {0xFF, 0x00, 0x00}, "black": {0x00, 0x00, 0x00},
	}
	if c, ok := m[strings.ToLower(team)]; ok {
		return c[0], c[1], c[2]
	}
	return 0xFF, 0xFF, 0xFF
}

// ShopPrice returns the currency ("Iron", "Gold" or "Emerald") and the price (at least 1)
// configured under shop.<key>.
func ShopPrice(key string) (currency string, price int) {
	m, ok := cfg.Get("shop."+key, nil).(map[string]any)
	if !ok {
		return "Iron", 1
	}
	cur, _ := m["currency"].(string)
	if cur != "Iron" && cur != "Gold" && cur != "Emerald" {
		cur = "Iron"
	}
	switch v := m["price"].(type) {
	case int:
		price = v
	case int64:
		price = int(v)
	case float64:
		price = int(v)
	}
	if price < 1 {
		price = 1
	}
	return cur, price
}

// CurrencyColor returns the chat color used to display a currency.
func CurrencyColor(cur string) string {
	switch cur {
	case "Gold":
		return Gold
	case "Emerald":
		return DarkGreen
	}
	return White
}
