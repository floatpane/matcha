//go:build unix

package view

import (
	"os"

	"golang.org/x/sys/unix"
)

// ttyCellPixels reads the pixel size of one terminal cell from the kernel
// window size (TIOCGWINSZ). Terminals that do not report pixel sizes return
// zeros, in which case ok is false.
func ttyCellPixels() (w, h int, ok bool) {
	for _, f := range []*os.File{os.Stdout, os.Stdin, os.Stderr} {
		ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
		if err != nil || ws.Col == 0 || ws.Row == 0 || ws.Xpixel == 0 || ws.Ypixel == 0 {
			continue
		}
		w, h = int(ws.Xpixel)/int(ws.Col), int(ws.Ypixel)/int(ws.Row)
		if w > 0 && h > 0 {
			return w, h, true
		}
	}
	return 0, 0, false
}
