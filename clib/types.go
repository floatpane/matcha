package clib

// ImageConvertResult holds the output of DecodeToPNG.
type ImageConvertResult struct {
	PNGData []byte
	Width   int
	Height  int
}
