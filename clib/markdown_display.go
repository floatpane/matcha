//go:build cgo

package clib

/*
#include "md4c.h"
#include "md4c-html.h"
#include <stdlib.h>
#include <string.h>

typedef struct {
    char* data;
    size_t len;
    size_t cap;
} MdDisplayBuf;

static void md_display_output_cb(const MD_CHAR* text, MD_SIZE size, void* userdata) {
    MdDisplayBuf* b = (MdDisplayBuf*)userdata;
    size_t needed = b->len + size;
    if (needed > b->cap) {
        size_t newcap = b->cap ? b->cap * 2 : 4096;
        while (newcap < needed) newcap *= 2;
        char* grown = (char*)realloc(b->data, newcap);
        if (!grown) return;
        b->data = grown;
        b->cap = newcap;
    }
    memcpy(b->data + b->len, text, size);
    b->len += size;
}

// md4c_to_html_hardbreaks is md4c_to_html with single newlines kept as
// line breaks, which is how plain-text email is meant to be read.
static char* md4c_to_html_hardbreaks(const char* input, size_t input_len, size_t* out_len) {
    MdDisplayBuf buf = {0};
    buf.cap = input_len * 2;
    if (buf.cap < 256) buf.cap = 256;
    buf.data = (char*)malloc(buf.cap);
    if (!buf.data) return NULL;

    unsigned parser_flags = MD_FLAG_PERMISSIVEAUTOLINKS |
                            MD_FLAG_TABLES |
                            MD_FLAG_STRIKETHROUGH |
                            MD_FLAG_TASKLISTS |
                            MD_FLAG_HARD_SOFT_BREAKS;

    int ret = md_html(input, (MD_SIZE)input_len, md_display_output_cb, &buf,
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
import (
	"log"
	"unsafe"
)

// MarkdownToHTMLHardBreaks converts Markdown to HTML like MarkdownToHTML but
// keeps every newline as a line break. Use it to display plain-text email,
// whose line breaks are meaningful (signatures, addresses, lists).
func MarkdownToHTMLHardBreaks(md []byte) []byte {
	if len(md) == 0 {
		return nil
	}

	cInput := C.CBytes(md)
	defer C.free(cInput)

	var outLen C.size_t
	result := C.md4c_to_html_hardbreaks((*C.char)(cInput), C.size_t(len(md)), &outLen)
	if result == nil {
		log.Printf("markdown: md4c_to_html failed, falling back to escaped plain-text HTML")
		return markdownPlainTextHTML(md)
	}
	defer C.free(unsafe.Pointer(result))

	return C.GoBytes(unsafe.Pointer(result), C.int(outLen))
}
