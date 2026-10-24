//go:build dev

package site

import (
	"fmt"
	"os"
	"testing"
)

func TestDump(t *testing.T) {
	f, err := os.Open("../atlas/embedded/site.bin")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	m := s.Manifesto
	fmt.Printf("lead %d spans (hi: ", len(m.Lead))
	for _, sp := range m.Lead {
		if sp.Hi {
			fmt.Printf("%q ", sp.Text)
		}
	}
	fmt.Printf("), body %d chars\ntitle %q signature %q date %q\n", len(m.Body), m.Title, m.Signature, m.Date)
	for i, p := range m.Paras {
		n, hi := 0, 0
		for _, sp := range p {
			n += len(sp.Text)
			if sp.Hi {
				hi++
			}
		}
		fmt.Printf("  para %2d: %4d chars, %d highlights\n", i, n, hi)
	}
	for _, l := range s.Luminaries {
		fmt.Printf("lum %-16s @%-16s verified=%v palette=%d cite=%d chars\n", l.Name, l.Handle, l.Verified, len(l.Art.Palette), len(l.Cite))
	}
	for _, sp := range s.Spotlights {
		fmt.Printf("spot %-14s @%-14s #%d %s quote=%d bio=%d %s\n", sp.Name, sp.Handle, sp.Snark, sp.Runtime, len(sp.Quote), len(sp.Bio), sp.Video)
	}
	fmt.Println("slots", s.Slots)
}

func fmtLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
