//go:build !unix

package view

// ttyCellPixels is unavailable on platforms without TIOCGWINSZ.
func ttyCellPixels() (w, h int, ok bool) {
	return 0, 0, false
}
