//go:build cgo

package clib

/*
#include "htmlconv.h"
#include <stdlib.h>
*/
import "C"

import (
	"html"
	"unsafe"
)

// HTMLToElements parses HTML and returns structured elements.
// This is a single-pass C parser that replaces goquery-based DOM parsing.
func HTMLToElements(doc string) ([]HTMLElement, bool) {
	if len(doc) == 0 {
		return nil, true
	}

	cHTML := C.CString(doc)
	defer C.free(unsafe.Pointer(cHTML))

	result := C.html_to_elements(cHTML, C.size_t(len(doc)))
	if result.ok == 0 {
		return nil, false
	}
	defer C.free_html_result(&result) //nolint:gocritic

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
			Type:  int(ce._type),
			Style: int(ce.style),
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
		unescapeAttributes(&elements[i])
	}

	return elements, true
}

// unescapeAttributes decodes character references in the attribute values
// the C parser copies verbatim. The sanitizer re-serializes every attribute
// HTML-escaped, so without this a link or image URL "?a=1&b=2" would be
// requested as "?a=1&amp;b=2". Text content is decoded by the parser itself.
func unescapeAttributes(e *HTMLElement) {
	switch e.Type {
	case HElemLink, HElemCode:
		e.Attr1 = html.UnescapeString(e.Attr1) // href, language
	case HElemImage:
		e.Attr1 = html.UnescapeString(e.Attr1) // src
		e.Attr2 = html.UnescapeString(e.Attr2) // alt
	case HElemBlockquote:
		e.Attr1 = html.UnescapeString(e.Attr1) // cite
	}
}
