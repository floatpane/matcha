//go:build !windows

package view

import "golang.org/x/sys/unix"

// getCellHeightFromFd attempts to get the terminal cell height from a file descriptor.
// Returns 0 if it fails or if pixel dimensions are not available.
func getCellHeightFromFd(fd int) int {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		return 0
	}

	// ws.Row = number of character rows
	// ws.Ypixel = height in pixels
	// Some terminals don't report pixel dimensions (return 0)
	if ws.Row > 0 && ws.Ypixel > 0 {
		cellHeight := int(ws.Ypixel) / int(ws.Row)
		if cellHeight > 0 {
			debugImageProtocol("terminal cell height: %d pixels (rows=%d, ypixel=%d, fd=%d)", cellHeight, ws.Row, ws.Ypixel, fd)
			return cellHeight
		}
	}

	// Terminal reported dimensions but no pixel info - this is common
	if ws.Row > 0 && ws.Ypixel == 0 {
		debugImageProtocol("terminal fd=%d has rows=%d but no pixel info (ypixel=0)", fd, ws.Row)
	}

	return 0
}
