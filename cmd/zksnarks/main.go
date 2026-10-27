// Command zksnarks is an unofficial terminal explorer for the zkSNARKs collection. All
// 10,000 artworks ship inside the binary, so browsing never tells the server which tokens
// you look at.
//
//	zksnarks                  launch explorer
//	zksnarks 7724             print the card of zkSNARK #7724
//	zksnarks random           print the card of a random zkSNARK
//	zksnarks bids             print the zkSNARKs with open bids
//	zksnarks sales            print the latest sales
//	zksnarks version          print the version
//	zksnarks help             show the help
//	zksnarks print 1 2 3      print thumbnails and cards to stdout
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	xterm "github.com/charmbracelet/x/term"

	"github.com/dangerous-person/zksnarks-cli/internal/art"
	"github.com/dangerous-person/zksnarks-cli/internal/atlas"
	"github.com/dangerous-person/zksnarks-cli/internal/atlas/embedded"
	"github.com/dangerous-person/zksnarks-cli/internal/market"
	"github.com/dangerous-person/zksnarks-cli/internal/media"
	"github.com/dangerous-person/zksnarks-cli/internal/term"
	"github.com/dangerous-person/zksnarks-cli/internal/ui"
)

var version = "0.1.0-dev"

func main() {
	fs := flag.NewFlagSet("zksnarks", flag.ContinueOnError)
	offline := fs.Bool("offline", false, "make no network requests; prices come from the build")
	colorFlag := fs.String("color", "auto", "colour profile: auto, truecolor or 256")
	glyphFlag := fs.String("glyph", "auto", "cell split glyph: auto, half (▀) or five-eighths (▅, for Terminal.app)")
	query := fs.String("q", "", "start with this search query")
	mediaDir := fs.String("media", "", "read the media from this folder instead of the copy inside the program: the front page's still (front.zkv), the trailer (trailer/montage.json), the spotlight interviews (NAME.zkv and NAME.mp3) and the about page's clip (noisebody.zkv)")
	showVersion := fs.Bool("version", false, "print the version")
	fs.SetOutput(io.Discard)
	if err := fs.Parse(os.Args[1:]); err == flag.ErrHelp {
		printHelp(os.Stdout, *colorFlag, *glyphFlag)
		return
	} else if err != nil {
		fmt.Fprintln(os.Stderr, "zksnarks:", err)
		printHelp(os.Stderr, *colorFlag, *glyphFlag)
		os.Exit(2)
	}
	if *showVersion {
		fmt.Println("zksnarks", version)
		return
	}
	profile := term.Color(*colorFlag, os.Getenv)
	glyph := term.Glyph(*glyphFlag, os.Getenv)
	if fs.Arg(0) == "print" {
		pf := flag.NewFlagSet("print", flag.ExitOnError)
		card := pf.Bool("card", false, "print the card: the big art with the traits and the market next to it")
		pf.Parse(fs.Args()[1:])
		if err := printTokens(pf.Args(), *card, profile, glyph, *offline); err != nil {
			fmt.Fprintln(os.Stderr, "zksnarks:", err)
			os.Exit(1)
		}
		return
	}
	commands := map[string]func(colorprofile.Profile, art.Glyph, bool) error{"random": printRandom, "bids": printBids, "sales": printSales}
	if cmd := fs.Arg(0); commands[cmd] != nil || cmd == "help" || cmd == "version" {
		if fs.NArg() > 1 {
			fmt.Fprintf(os.Stderr, "zksnarks: %s takes no arguments\n", cmd)
			os.Exit(2)
		}
		switch cmd {
		case "help":
			printHelp(os.Stdout, *colorFlag, *glyphFlag)
			return
		case "version":
			fmt.Println("zksnarks", version)
			return
		}
		if err := commands[cmd](profile, glyph, *offline); err != nil {
			fmt.Fprintln(os.Stderr, "zksnarks:", err)
			os.Exit(1)
		}
		return
	}
	if fs.NArg() > 0 {
		if _, err := strconv.Atoi(strings.TrimPrefix(fs.Arg(0), "#")); err != nil {
			fmt.Fprintf(os.Stderr, "zksnarks: unknown command %q\n", fs.Arg(0))
			printHelp(os.Stderr, *colorFlag, *glyphFlag)
			os.Exit(2)
		}
		if err := printTokens(fs.Args(), true, profile, glyph, *offline); err != nil {
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
	if !xterm.IsTerminal(os.Stdout.Fd()) || os.Getenv("TERM") == "dumb" {
		needTerminal()
	}
	opts := []tea.ProgramOption{tea.WithColorProfile(profile)}
	if !xterm.IsTerminal(os.Stdin.Fd()) {
		in, _, err := tea.OpenTTY()
		if err != nil {
			needTerminal()
		}
		opts = append(opts, tea.WithInput(in))
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
	m := ui.New(ui.Options{Atlas: a, Site: content, Glyph: glyph, Color: profile, NoColor: os.Getenv("NO_COLOR") != "", Client: market.New(*offline), State: loadState(), Query: *query, Media: files})
	final, err := tea.NewProgram(m, opts...).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "zksnarks:", err)
		os.Exit(1)
	}
	if fm, ok := final.(*ui.Model); ok {
		saveState(fm.SavedState())
	}
}

func printHelp(f *os.File, color, glyph string) {
	const grey, quiet, reset = "\x1b[38;2;139;139;148m", "\x1b[38;2;63;63;71m", "\x1b[0m"
	profile, cols := colorprofile.NoTTY, 0
	if xterm.IsTerminal(f.Fd()) && os.Getenv("TERM") != "dumb" {
		profile = colorprofile.Ascii
		if os.Getenv("NO_COLOR") == "" {
			profile = term.Color(color, os.Getenv)
		}
		cols, _, _ = xterm.GetSize(f.Fd())
	}
	about := "Unofficial terminal explorer for the zkSNARKs collection."
	body := []string{"\x1b[1mZKSNARKS_" + reset, grey + about + reset, ""}
	w := len(about)
	for _, c := range [][2]string{
		{"zksnarks", "launch explorer"},
		{"zksnarks 7724", "print the card of zkSNARK #7724"},
		{"zksnarks random", "print the card of a random zkSNARK"},
		{"zksnarks bids", "print the zkSNARKs with open bids"},
		{"zksnarks sales", "print the latest sales"},
		{"zksnarks version", "print the version"},
		{"zksnarks help", "show this help"},
	} {
		body = append(body, fmt.Sprintf("%-22s%s%s%s", c[0], grey, c[1], reset))
		w = max(w, 22+len(c[1]))
	}
	const logoW, logoGap = 20, 3
	var logo []string
	if profile != colorprofile.NoTTY && cols >= logoW+logoGap+w {
		logo = ui.Logo(term.Glyph(glyph, os.Getenv))
		w += logoW + logoGap
	}
	rule := quiet + strings.Repeat("-", w) + reset
	var b strings.Builder
	b.WriteString("\n" + rule + "\n\n")
	for i := range max(len(logo), len(body)) {
		if logo != nil {
			l := strings.Repeat(" ", logoW)
			if i < len(logo) {
				l = logo[i]
			}
			b.WriteString(l + strings.Repeat(" ", logoGap))
		}
		if i < len(body) {
			b.WriteString(body[i])
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + rule + "\n\n")
	fmt.Fprint(&colorprofile.Writer{Forward: f, Profile: profile}, b.String())
}

func needTerminal() {
	msg := "zksnarks needs an interactive terminal.\ntry: zksnarks 7724   (prints the card of one token)\n"
	if runtime.GOOS == "windows" {
		msg += "in Git Bash run: winpty zksnarks\n"
	}
	fmt.Fprint(os.Stderr, msg)
	os.Exit(1)
}

func printTokens(args []string, card bool, profile colorprofile.Profile, glyph art.Glyph, offline bool) error {
	a, err := embedded.Load()
	if err != nil {
		return err
	}
	var ids []int
	for _, s := range args {
		id, err := strconv.Atoi(strings.TrimPrefix(s, "#"))
		if err != nil || id < 1 || id > len(a.Tokens) {
			return fmt.Errorf("%q is not a token number", s)
		}
		ids = append(ids, id)
	}
	if card {
		return printCards(a, ids, profile, glyph, offline)
	}
	w := &colorprofile.Writer{Forward: os.Stdout, Profile: profile}
	q := term.NewQuantiser(profile)
	for _, id := range ids {
		t := a.Tokens[id-1]
		fmt.Fprintf(w, "#%d  %s\nhttps://zilkroad.com%s\n", t.ID, strings.Join(t.Traits[:5], " · "), t.Img)
		fmt.Fprintln(w, q.Apply(strings.Join(art.Thumb(t.Art, glyph), "\n")))
		fmt.Fprintln(w)
	}
	return nil
}

func printRandom(profile colorprofile.Profile, glyph art.Glyph, offline bool) error {
	a, err := embedded.Load()
	if err != nil {
		return err
	}
	return printCards(a, []int{rand.IntN(len(a.Tokens)) + 1}, profile, glyph, offline)
}

func printCards(a *atlas.Atlas, ids []int, profile colorprofile.Profile, glyph art.Glyph, offline bool) error {
	m, client, err := printModel(a, profile, glyph, offline)
	if err != nil {
		return err
	}
	data := liveData(client)
	for _, id := range ids {
		lines, _ := m.PrintCard(id, data...)
		printPanel(profile, lines)
	}
	return nil
}

func printBids(profile colorprofile.Profile, glyph art.Glyph, offline bool) error {
	a, err := embedded.Load()
	if err != nil {
		return err
	}
	m, client, err := printModel(a, profile, glyph, offline)
	if err != nil {
		return err
	}
	printPanel(profile, m.PrintBids(termCols(), liveData(client)...))
	return nil
}

func printSales(profile colorprofile.Profile, glyph art.Glyph, offline bool) error {
	if offline {
		return errors.New("sales come from zilkroad, run it without -offline")
	}
	a, err := embedded.Load()
	if err != nil {
		return err
	}
	m, client, err := printModel(a, profile, glyph, offline)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sales, err := client.Sales(ctx)
	if err != nil {
		return fmt.Errorf("can't reach zilkroad: %w", err)
	}
	var data []any
	if s, _, err := client.Stats(ctx, ""); err == nil {
		data = append(data, s)
	}
	if r, _, err := client.Rate(ctx, ""); err == nil {
		data = append(data, r)
	}
	printPanel(profile, m.PrintSales(termCols(), sales, data...))
	return nil
}

func printModel(a *atlas.Atlas, profile colorprofile.Profile, glyph art.Glyph, offline bool) (*ui.Model, *market.Client, error) {
	content, err := embedded.LoadSite()
	if err != nil {
		return nil, nil, err
	}
	client := market.New(offline)
	return ui.New(ui.Options{Atlas: a, Site: content, Glyph: glyph, Color: profile, NoColor: os.Getenv("NO_COLOR") != "", Client: client, State: loadState()}), client, nil
}

func liveData(client *market.Client) []any {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var data []any
	if s, _, err := client.Snapshot(ctx, ""); err == nil {
		data = append(data, s)
	}
	if r, _, err := client.Rate(ctx, ""); err == nil {
		data = append(data, r)
	}
	return data
}

func termCols() int {
	if xterm.IsTerminal(os.Stdout.Fd()) {
		if w, _, err := xterm.GetSize(os.Stdout.Fd()); err == nil {
			return w
		}
	}
	return 0
}

func printPanel(profile colorprofile.Profile, lines []string) {
	w := &colorprofile.Writer{Forward: os.Stdout, Profile: profile}
	fmt.Fprintln(w, term.NewQuantiser(profile).Apply(strings.Join(lines, "\n")))
	fmt.Fprintln(w)
}

// stateFile is the remembered theme, in the XDG config directory.
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
