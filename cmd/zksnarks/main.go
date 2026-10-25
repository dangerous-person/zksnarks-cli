// Command zksnarks is an unofficial terminal explorer for the zkSNARKs collection. All
// 10,000 artworks ship inside the binary, so browsing never tells the server which tokens
// you look at.
//
//	zksnarks                  explore
//	zksnarks print 1 2 3      print thumbnails and cards to stdout
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/atlas/embedded"
	"github.com/dangerous-person/zksnarks-cli/internal/market"
	"github.com/dangerous-person/zksnarks-cli/internal/media"
	"github.com/dangerous-person/zksnarks-cli/internal/term"
	"github.com/dangerous-person/zksnarks-cli/internal/ui"
)

var version = "0.1.0-dev"

func main() {
	fs := flag.NewFlagSet("zksnarks", flag.ExitOnError)
	offline := fs.Bool("offline", false, "make no network requests; prices come from the build")
	colorFlag := fs.String("color", "auto", "colour profile: auto, truecolor or 256")
	glyphFlag := fs.String("glyph", "auto", "cell split glyph: auto, half (▀) or five-eighths (▅, for Terminal.app)")
	query := fs.String("q", "", "start with this search query")
	mediaDir := fs.String("media", "", "read the media from this folder instead of the copy inside the program: the front page's still (front.zkv), the trailer (trailer/montage.json), the spotlight interviews (NAME.zkv and NAME.mp3) and the about page's clip (noisebody.zkv)")
	showVersion := fs.Bool("version", false, "print the version")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: zksnarks [flags]\n       zksnarks [flags] print [-card] N...\n\n")
		fs.PrintDefaults()
	}
	fs.Parse(os.Args[1:])
	if *showVersion {
		fmt.Println("zksnarks", version)
		return
	}
	profile := term.Color(*colorFlag, os.Getenv)
	glyph := term.Glyph(*glyphFlag, os.Getenv)
	if fs.Arg(0) == "print" {
		if err := printTokens(fs.Args()[1:], profile, glyph); err != nil {
			fmt.Fprintln(os.Stderr, "zksnarks:", err)
			os.Exit(1)
		}
		return
	}
	files := media.FS
	if *mediaDir != "" {
		if st, err := os.Stat(*mediaDir); err != nil || !st.IsDir() {
			fmt.Fprintln(os.Stderr, "zksnarks: -media: there is no folder", *mediaDir)
			os.Exit(1)
		}
		files = os.DirFS(*mediaDir)
	}
	a, err := embedded.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "zksnarks:", err)
		os.Exit(1)
	}
	content, err := embedded.LoadSite()
	if err != nil {
		fmt.Fprintln(os.Stderr, "zksnarks:", err)
		os.Exit(1)
	}
	st := loadState()
	if *query != "" {
		st.Query = *query
	}
	m := ui.New(ui.Options{Atlas: a, Site: content, Glyph: glyph, Color: profile, NoColor: os.Getenv("NO_COLOR") != "", Client: market.New(*offline), State: st, Media: files})
	final, err := tea.NewProgram(m, tea.WithColorProfile(profile)).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "zksnarks:", err)
		os.Exit(1)
	}
	if fm, ok := final.(*ui.Model); ok {
		saveState(fm.SavedState())
	}
}

func printTokens(args []string, profile colorprofile.Profile, glyph art.Glyph) error {
	pf := flag.NewFlagSet("print", flag.ExitOnError)
	card := pf.Bool("card", false, "print the 52x26 card next to the thumbnail")
	pf.Parse(args)
	a, err := embedded.Load()
	if err != nil {
		return err
	}
	w := &colorprofile.Writer{Forward: os.Stdout, Profile: profile}
	q := term.NewQuantiser(profile)
	for _, s := range pf.Args() {
		id, err := strconv.Atoi(strings.TrimPrefix(s, "#"))
		if err != nil || id < 1 || id > len(a.Tokens) {
			return fmt.Errorf("%q is not a token number", s)
		}
		t := a.Tokens[id-1]
		fmt.Fprintf(w, "#%d  %s\nhttps://zilkroad.com%s\n", t.ID, strings.Join(t.Traits[:5], " · "), t.Img)
		thumb := art.Thumb(t.Art, glyph)
		if !*card {
			fmt.Fprintln(w, q.Apply(strings.Join(thumb, "\n")))
			fmt.Fprintln(w)
			continue
		}
		cardLines := art.Card(t.Art)
		for i, c := range cardLines {
			left := strings.Repeat(" ", art.ThumbW)
			if i < len(thumb) {
				left = thumb[i]
			}
			fmt.Fprintln(w, q.Apply(left+"    "+c))
		}
		fmt.Fprintln(w)
	}
	return nil
}

// stateFile is the remembered query, view and theme, in the XDG config directory.
func stateFile() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "zksnarks", "state.json")
}

func loadState() ui.State {
	var st ui.State
	if p := stateFile(); p != "" {
		if data, err := os.ReadFile(p); err == nil {
			json.Unmarshal(data, &st)
		}
	}
	return st
}

func saveState(st ui.State) {
	p := stateFile()
	if p == "" {
		return
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(p), 0o755) == nil {
		os.WriteFile(p, append(data, '\n'), 0o644)
	}
}
