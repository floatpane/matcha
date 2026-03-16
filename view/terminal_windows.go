//go:build windows

package view

// getCellHeightFromFd is not supported on Windows.
// Returns 0 to fall back to the default cell height.
func getCellHeightFromFd(fd int) int {
	return 0
}
