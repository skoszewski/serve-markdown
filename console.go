package main

import (
	"fmt"
	"os"
)

// Colour escapes, empty unless stdout is a terminal.
var (
	colorReset  = ""
	colorRed    = ""
	colorYellow = ""
	colorCyan   = ""
)

func init() {
	info, err := os.Stdout.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return
	}
	colorReset = "\033[0m"
	colorRed = "\033[0;31m"
	colorYellow = "\033[0;33m"
	colorCyan = "\033[0;36m"
}

// logInfo writes one human-readable line to stdout.
func logInfo(format string, args ...any) {
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}

// logError writes one error line to stderr.
func logError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// printBanner writes title between two cyan rules.
func printBanner(title string) {
	rule := colorCyan + "==========================================" + colorReset
	logInfo("%s", rule)
	logInfo("%s%s%s", colorCyan, title, colorReset)
	logInfo("%s", rule)
}
