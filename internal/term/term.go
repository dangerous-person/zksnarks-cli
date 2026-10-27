// Package term decides how to draw for the terminal the app runs in: the colour profile, the
// palette indices for terminals without truecolor, and the glyph used to split a cell
// between two art pixels.
package term

import (
	"runtime"
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
	if runtime.GOOS == "windows" {
		return colorprofile.TrueColor
	}
	if strings.HasPrefix(env("TERM"), "screen") && env("TMUX") == "" {
		return colorprofile.ANSI256
	}
	if trueColor(env) {
		return colorprofile.TrueColor
	}
	return colorprofile.ANSI256
}

func trueColor(env Env) bool {
	switch strings.ToLower(env("COLORTERM")) {
	case "truecolor", "24bit":
		return true
	}
	if env("WT_SESSION") != "" || env("TERMINAL_EMULATOR") == "JetBrains-JediTerm" || env("LC_TERMINAL") == "iTerm2" || otherTerminal(env) {
		return true
	}
	switch program(env) {
	case "ghostty", "iTerm.app", "WezTerm", "vscode", "Hyper", "rio", "Tabby", "WarpTerminal":
		return true
	case "Apple_Terminal":
		if appleTerminalTrueColor() {
			return true
		}
	}
	t := env("TERM")
	return strings.Contains(t, "kitty") || strings.Contains(t, "ghostty") || strings.Contains(t, "direct") || strings.Contains(t, "alacritty")
}

func otherTerminal(env Env) bool {
	for _, k := range []string{"KITTY_WINDOW_ID", "ALACRITTY_WINDOW_ID", "WEZTERM_EXECUTABLE", "GHOSTTY_RESOURCES_DIR"} {
		if env(k) != "" {
			return true
		}
	}
	return false
}

func program(env Env) string {
	p := env("TERM_PROGRAM")
	if (p == "tmux" || p == "screen") && env("__CFBundleIdentifier") == "com.apple.Terminal" {
		p = "Apple_Terminal"
	}
	if p == "Apple_Terminal" && otherTerminal(env) {
		return ""
	}
	return p
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
	if program(env) == "Apple_Terminal" {
		return art.LowerFiveEighths
	}
	return art.UpperHalf
}
