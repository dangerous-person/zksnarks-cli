package term

import (
	"strconv"
	"strings"
	"syscall"
)

// appleTerminalTrueColor reports whether Terminal.app can draw 24-bit colour: it does on
// macOS 26 (Darwin 25), checked on 26.5, and approximates it on earlier versions.
func appleTerminalTrueColor() bool {
	rel, err := syscall.Sysctl("kern.osrelease")
	if err != nil {
		return false
	}
	major, err := strconv.Atoi(strings.SplitN(rel, ".", 2)[0])
	return err == nil && major >= 25
}
