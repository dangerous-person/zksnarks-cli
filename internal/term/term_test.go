package term

import (
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
)

func lookup(vars string) Env {
	m := map[string]string{}
	for _, kv := range strings.Fields(vars) {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return func(k string) string { return m[k] }
}

func TestColor(t *testing.T) {
	apple := colorprofile.ANSI256
	if appleTerminalTrueColor() {
		apple = colorprofile.TrueColor
	}
	const tc, c256 = colorprofile.TrueColor, colorprofile.ANSI256
	for _, c := range []struct {
		flag, env string
		want      colorprofile.Profile
	}{
		{"truecolor", "TERM=screen", tc},
		{"24bit", "", tc},
		{"256", "COLORTERM=truecolor", c256},
		{"auto", "", c256},
		{"auto", "TERM=xterm-256color", c256},
		{"auto", "TERM=xterm-256color COLORTERM=truecolor", tc},
		{"auto", "TERM=xterm-256color COLORTERM=24BIT", tc},
		{"auto", "TERM=xterm-256color NO_COLOR=1", c256},
		{"auto", "TERM=xterm-256color COLORTERM=truecolor NO_COLOR=1", tc},
		{"auto", "TERM=xterm-256color WT_SESSION=5f1e", tc},
		{"auto", "TERM=xterm-256color TERMINAL_EMULATOR=JetBrains-JediTerm", tc},
		{"auto", "TERM=xterm-256color TERMINAL_EMULATOR=other", c256},
		{"auto", "TERM=xterm-256color LC_TERMINAL=iTerm2", tc},
		{"auto", "TERM=xterm-256color TERM_PROGRAM=iTerm.app", tc},
		{"auto", "TERM=xterm-256color TERM_PROGRAM=ghostty", tc},
		{"auto", "TERM=xterm-256color TERM_PROGRAM=Apple_Terminal", apple},
		{"auto", "TERM=xterm-kitty TERM_PROGRAM=Apple_Terminal KITTY_WINDOW_ID=1", tc},
		{"auto", "TERM=xterm-256color TERM_PROGRAM=Apple_Terminal ALACRITTY_WINDOW_ID=1", tc},
		{"auto", "TERM=xterm-256color TERM_PROGRAM=Apple_Terminal WEZTERM_EXECUTABLE=/x", tc},
		{"auto", "TERM=xterm-256color TERM_PROGRAM=Apple_Terminal GHOSTTY_RESOURCES_DIR=/x", tc},
		{"auto", "TERM=xterm-kitty", tc},
		{"auto", "TERM=xterm-ghostty", tc},
		{"auto", "TERM=alacritty", tc},
		{"auto", "TERM=xterm-direct", tc},
		{"auto", "TERM=screen", c256},
		{"auto", "TERM=screen-256color COLORTERM=truecolor", c256},
		{"auto", "TERM=screen.xterm-256color WT_SESSION=1", c256},
		{"auto", "TERM=screen-256color TMUX=/tmp/t,1,0 TERM_PROGRAM=tmux", c256},
		{"auto", "TERM=screen-256color TMUX=/tmp/t,1,0 TERM_PROGRAM=tmux COLORTERM=truecolor", tc},
		{"auto", "TERM=tmux-256color TMUX=/tmp/t,1,0 TERM_PROGRAM=tmux", c256},
		{"auto", "TERM=tmux-256color TMUX=/tmp/t,1,0 TERM_PROGRAM=tmux KITTY_WINDOW_ID=3", tc},
		{"auto", "TERM=tmux-256color TMUX=/tmp/t,1,0 TERM_PROGRAM=tmux WT_SESSION=1", tc},
		{"auto", "TERM=tmux-256color TMUX=/tmp/t,1,0 TERM_PROGRAM=tmux LC_TERMINAL=iTerm2", tc},
		{"auto", "TERM=tmux-direct TMUX=/tmp/t,1,0", tc},
	} {
		if got := Color(c.flag, lookup(c.env)); got != c.want {
			t.Errorf("-color %s with %q: got %v, want %v", c.flag, c.env, got, c.want)
		}
	}
}

func TestGlyph(t *testing.T) {
	for _, c := range []struct {
		flag, env string
		want      art.Glyph
	}{
		{"auto", "", art.UpperHalf},
		{"auto", "TERM_PROGRAM=Apple_Terminal", art.LowerFiveEighths},
		{"auto", "TERM_PROGRAM=Apple_Terminal KITTY_WINDOW_ID=1 TERM=xterm-kitty", art.UpperHalf},
		{"auto", "TERM_PROGRAM=Apple_Terminal ALACRITTY_WINDOW_ID=1", art.UpperHalf},
		{"auto", "TERM_PROGRAM=Apple_Terminal WEZTERM_EXECUTABLE=/x", art.UpperHalf},
		{"auto", "TERM_PROGRAM=Apple_Terminal GHOSTTY_RESOURCES_DIR=/x", art.UpperHalf},
		{"auto", "TERM_PROGRAM=Apple_Terminal WT_SESSION=1", art.LowerFiveEighths},
		{"auto", "TERM_PROGRAM=ghostty", art.UpperHalf},
		{"auto", "TERM_PROGRAM=tmux __CFBundleIdentifier=com.apple.Terminal", art.LowerFiveEighths},
		{"auto", "TERM_PROGRAM=tmux __CFBundleIdentifier=com.mitchellh.ghostty", art.UpperHalf},
		{"auto", "TERM_PROGRAM=tmux", art.UpperHalf},
		{"half", "TERM_PROGRAM=Apple_Terminal", art.UpperHalf},
		{"five-eighths", "TERM_PROGRAM=ghostty", art.LowerFiveEighths},
	} {
		if got := Glyph(c.flag, lookup(c.env)); got != c.want {
			t.Errorf("-glyph %s with %q: got %v, want %v", c.flag, c.env, got, c.want)
		}
	}
}
