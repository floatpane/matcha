//go:build cgo

package clib

/*
#include "htmlconv.h"
#include <stdlib.h>
*/
import "C"
import "unsafe"

// HTMLToElements parses HTML and returns structured elements.
// This is a single-pass C parser that replaces goquery-based DOM parsing.
func HTMLToElements(html string) ([]HTMLElement, bool) {
	if len(html) == 0 {
		return nil, true
	}

	cHTML := C.CString(html)
	defer C.free(unsafe.Pointer(cHTML))

	result := C.html_to_elements(cHTML, C.size_t(len(html)))
	if result.ok == 0 {
		return nil, false
	}
	defer C.free_html_result(&result)

	count := int(result.count)
	if count == 0 {
		return nil, true
	}

	elements := make([]HTMLElement, count)

	// Access the C array via pointer arithmetic.
	cElems := (*[1 << 20]C.HTMLElement)(unsafe.Pointer(result.elements))[:count:count]

	for i := 0; i < count; i++ {
		ce := &cElems[i]
		elements[i] = HTMLElement{
			Type: int(ce._type),
		}
		if ce.text != nil {
			elements[i].Text = C.GoString(ce.text)
		}
		if ce.attr1 != nil {
			elements[i].Attr1 = C.GoString(ce.attr1)
		}
		if ce.attr2 != nil {
			elements[i].Attr2 = C.GoString(ce.attr2)
		}
	}

	return elements, true
}
