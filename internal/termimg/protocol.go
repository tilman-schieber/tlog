// Package termimg draws a picture in a terminal that can show one, in
// whichever of the three common image protocols it speaks. It knows nothing of
// notes: it is handed a picture and a box of cells, and answers with the bytes
// to write.
//
// Which protocol a terminal speaks is read from its environment where that is
// conclusive, and otherwise asked of the terminal when a picture is about to be
// shown — foot, for one, often runs as TERM=xterm-256color and says nothing
// else about itself. Asking happens only once the outliner has let go of the
// terminal, so the answer cannot be read as keys, and a terminal that cannot
// show a picture gets it handed to the system's opener instead, which is
// always right if not always pretty.
package termimg

import (
	"regexp"
	"strings"
)

// Protocol is a way of putting pixels in a terminal.
type Protocol int

const (
	// None: the terminal cannot show a picture, or tlog cannot tell.
	None Protocol = iota
	// Kitty is kitty's graphics protocol: kitty, ghostty, and others that
	// adopted it.
	Kitty
	// ITerm is iTerm2's inline images (OSC 1337): iTerm2, WezTerm, mintty.
	ITerm
	// Sixel is DEC's sixel graphics: foot, mlterm, contour, Konsole, Windows
	// Terminal, xterm built with it.
	Sixel
	// Ask: the environment does not say, so the terminal is asked when a
	// picture is shown.
	Ask
)

func (p Protocol) String() string {
	switch p {
	case Kitty:
		return "kitty"
	case ITerm:
		return "iterm"
	case Sixel:
		return "sixel"
	case Ask:
		return "ask"
	}
	return "none"
}

// Detect reads which protocol the terminal speaks from its environment, or
// answers Ask when the environment does not settle it.
// TLOG_IMAGES — kitty, iterm, sixel or none — overrides what is detected, for a
// terminal this does not know or one it gets wrong.
func Detect(getenv func(string) string) Protocol {
	switch strings.ToLower(strings.TrimSpace(getenv("TLOG_IMAGES"))) {
	case "kitty":
		return Kitty
	case "iterm", "iterm2":
		return ITerm
	case "sixel":
		return Sixel
	case "none", "off", "no":
		return None
	}

	term := strings.ToLower(getenv("TERM"))
	program := strings.ToLower(getenv("TERM_PROGRAM"))

	// Inside tmux or screen the escape sequences reach the multiplexer, not
	// the terminal, and getting them through needs passthrough that may be
	// off. The opener is the honest answer there.
	if getenv("TMUX") != "" || strings.HasPrefix(term, "screen") || strings.HasPrefix(term, "tmux") {
		return None
	}

	switch {
	case getenv("KITTY_WINDOW_ID") != "" || term == "xterm-kitty":
		return Kitty
	case program == "ghostty" || term == "xterm-ghostty" || getenv("GHOSTTY_RESOURCES_DIR") != "":
		return Kitty
	case program == "iterm.app" || program == "wezterm" || program == "mintty":
		return ITerm
	case term == "foot" || strings.HasPrefix(term, "foot-"):
		return Sixel
	case strings.HasPrefix(term, "mlterm") || strings.HasPrefix(term, "contour"):
		return Sixel
	case getenv("KONSOLE_VERSION") != "" || getenv("WT_SESSION") != "":
		return Sixel
	}
	return Ask
}

// Answer reads what a terminal said to the three questions probe asks: its
// name (XTVERSION), whether it takes a kitty picture, and its device
// attributes (DA1), where a 4 means sixel. It reports whether the answer is
// complete — DA1 comes last, because every terminal answers it and answers in
// order — and the protocol it implies.
func Answer(reply string) (Protocol, bool) {
	da1 := da1Re.FindStringSubmatch(reply)
	if da1 == nil {
		return None, false
	}
	name := strings.ToLower(reply)
	switch {
	case strings.Contains(name, "iterm2") || strings.Contains(name, "wezterm"):
		return ITerm, true
	case strings.Contains(reply, "\x1b_Gi=31;OK"):
		return Kitty, true
	}
	for _, attr := range strings.Split(da1[1], ";") {
		if attr == "4" {
			return Sixel, true
		}
	}
	return None, true
}

var da1Re = regexp.MustCompile(`\x1b\[\?([0-9;]*)c`)

// Questions is what probe writes: XTVERSION, a one-pixel kitty query that
// draws nothing, and DA1.
const Questions = "\x1b[>0q\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\\x1b[c"
