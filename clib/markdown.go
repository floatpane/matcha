//go:build cgo

package clib

/*
#include "md4c.h"
#include "md4c-html.h"
#include <stdlib.h>
#include <string.h>

// Buffer for collecting md4c-html output.
typedef struct {
    char* data;
    size_t len;
    size_t cap;
} MdBuf;

static void md_output_cb(const MD_CHAR* text, MD_SIZE size, void* userdata) {
    MdBuf* b = (MdBuf*)userdata;
    size_t needed = b->len + size;
    if (needed > b->cap) {
        size_t newcap = b->cap ? b->cap * 2 : 4096;
        while (newcap < needed) newcap *= 2;
        b->data = (char*)realloc(b->data, newcap);
        b->cap = newcap;
    }
    memcpy(b->data + b->len, text, size);
    b->len += size;
}

// md4c_to_html converts Markdown to HTML using md4c.
// Returns the HTML string (caller must free) and sets *out_len.
// Returns NULL on failure.
static char* md4c_to_html(const char* input, size_t input_len, size_t* out_len) {
    MdBuf buf = {0};
    buf.cap = input_len * 2;
    if (buf.cap < 256) buf.cap = 256;
    buf.data = (char*)malloc(buf.cap);
    if (!buf.data) return NULL;

    // Use permissive flags to handle raw HTML in emails.
    unsigned parser_flags = MD_FLAG_PERMISSIVEAUTOLINKS |
                            MD_FLAG_TABLES |
                            MD_FLAG_STRIKETHROUGH |
                            MD_FLAG_TASKLISTS;

    int ret = md_html(input, (MD_SIZE)input_len, md_output_cb, &buf,
                      parser_flags, 0);
    if (ret != 0) {
        free(buf.data);
        return NULL;
    }

    *out_len = buf.len;
    return buf.data;
}
*/
import "C"
import "unsafe"

// MarkdownToHTML converts Markdown bytes to HTML using md4c (C).
// This is significantly faster than goldmark for large documents.
func MarkdownToHTML(md []byte) []byte {
	if len(md) == 0 {
		return nil
	}

	cInput := C.CBytes(md)
	defer C.free(cInput)

	var outLen C.size_t
	result := C.md4c_to_html((*C.char)(cInput), C.size_t(len(md)), &outLen)
	if result == nil {
		return md // fallback to original on failure
	}
	defer C.free(unsafe.Pointer(result))

	return C.GoBytes(unsafe.Pointer(result), C.int(outLen))
}
