package ui

import (
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// input is a one-line text field with readline-style keys.
type input struct {
	r    []rune
	pos  int
	text string
}

func (in *input) set(s string) {
	in.r = []rune(s)
	in.pos = len(in.r)
	in.text = s
}

func (in *input) end() { in.pos = len(in.r) }

// key applies an editing key and reports whether the text changed.
func (in *input) key(msg tea.KeyPressMsg) bool {
	before := in.text
	switch msg.Keystroke() {
	case "backspace", "ctrl+h":
		if in.pos > 0 {
			in.r = append(in.r[:in.pos-1], in.r[in.pos:]...)
			in.pos--
		}
	case "delete":
		if in.pos < len(in.r) {
			in.r = append(in.r[:in.pos], in.r[in.pos+1:]...)
		}
	case "left", "ctrl+b":
		in.pos = max(0, in.pos-1)
	case "right", "ctrl+f":
		in.pos = min(len(in.r), in.pos+1)
	case "home", "ctrl+a":
		in.pos = 0
	case "end", "ctrl+e":
		in.pos = len(in.r)
	case "ctrl+w", "alt+backspace":
		i := in.pos
		for i > 0 && in.r[i-1] == ' ' {
			i--
		}
		for i > 0 && in.r[i-1] != ' ' {
			i--
		}
		in.r = append(in.r[:i], in.r[in.pos:]...)
		in.pos = i
	case "ctrl+u":
		in.r = append([]rune{}, in.r[in.pos:]...)
		in.pos = 0
	case "ctrl+k":
		in.r = in.r[:in.pos]
	default:
		if msg.Text == "" || msg.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModMeta|tea.ModSuper) != 0 {
			return false
		}
		var add []rune
		for _, c := range msg.Text {
			if unicode.IsPrint(c) {
				add = append(add, c)
			}
		}
		in.r = append(in.r[:in.pos], append(add, in.r[in.pos:]...)...)
		in.pos += len(add)
	}
	in.text = string(in.r)
	return in.text != before
}
