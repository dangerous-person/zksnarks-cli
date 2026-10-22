// Package term decides how to draw for the terminal the app runs in: the colour profile and
// the glyph used to split a cell between two art pixels.
package term

import (
	"strings"

	"github.com/charmbracelet/colorprofile"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
)

// Env is a lookup into the environment, os.Getenv in production.
type Env func(string) string

// Color returns the colour profile: truecolor where the terminal is known to support it,
// 256 colours otherwise. Bubble Tea's own detection would classify Terminal.app as 256
// colours, which turns the art's exact colours into approximations.
func Color(flag string, env Env) colorprofile.Profile {
	switch flag {
	case "truecolor", "24bit":
		return colorprofile.TrueColor
	case "256":
		return colorprofile.ANSI256
	}
	if env("NO_COLOR") != "" {
		return colorprofile.ASCII
	}
	switch strings.ToLower(env("COLORTERM")) {
	case "truecolor", "24bit":
		return colorprofile.TrueColor
	}
	switch env("TERM_PROGRAM") {
	case "ghostty", "iTerm.app", "WezTerm", "vscode", "Hyper", "rio", "Tabby", "WarpTerminal":
		return colorprofile.TrueColor
	case "Apple_Terminal":
		if appleTerminalTrueColor() {
			return colorprofile.TrueColor
		}
	}
	t := env("TERM")
	if strings.Contains(t, "kitty") || strings.Contains(t, "ghostty") || strings.Contains(t, "direct") || strings.Contains(t, "alacritty") {
		return colorprofile.TrueColor
	}
	return colorprofile.ANSI256
}

// Glyph returns the cell-splitting glyph. Terminal.app takes block glyphs from the font, and
// with its default fonts only ▅ splits a cell into equal halves; every other terminal tested
// draws ▀ to the cell edges itself.
func Glyph(flag string, env Env) art.Glyph {
	switch flag {
	case "half":
		return art.UpperHalf
	case "five-eighths":
		return art.LowerFiveEighths
	}
	if env("TERM_PROGRAM") == "Apple_Terminal" {
		return art.LowerFiveEighths
	}
	return art.UpperHalf
}
