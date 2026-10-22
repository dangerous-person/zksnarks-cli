//go:build !darwin

package term

func appleTerminalTrueColor() bool { return false }
