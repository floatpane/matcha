//go:build cgo

package clib

/*
#include "base64wrap.h"
#include <stdlib.h>
*/
import "C"
import "unsafe"

// WrapBase64 wraps base64-encoded data at 76 characters per line with \r\n
// separators, as required by MIME (RFC 2045).
func WrapBase64(data string) string {
	if len(data) == 0 {
		return ""
	}

	cData := C.CString(data)
	defer C.free(unsafe.Pointer(cData))

	var outLen C.size_t
	result := C.wrap_base64(cData, C.size_t(len(data)), &outLen)
	if result == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(result))

	return C.GoStringN(result, C.int(outLen))
}
