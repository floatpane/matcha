#ifndef MATCHA_HTMLCONV_H
#define MATCHA_HTMLCONV_H

#include <stddef.h>

// Element types found during HTML parsing.
enum {
    HELEM_TEXT       = 0,  // Plain text segment
    HELEM_H1         = 1,  // <h1> text
    HELEM_H2         = 2,  // <h2> text
    HELEM_LINK       = 3,  // <a href="...">text</a>
    HELEM_IMAGE      = 4,  // <img src="..." alt="...">
    HELEM_BLOCKQUOTE = 5,  // <blockquote> content (with optional cite/prev)
};

// HTMLElement represents a parsed element from the HTML document.
typedef struct {
    int type;
    char* text;     // Text content (h1/h2 text, link text, blockquote content)
    char* attr1;    // href for links, src for images, cite for blockquotes
    char* attr2;    // alt for images, prev_text for blockquotes (On...wrote:)
} HTMLElement;

// HTMLConvertResult holds the output of html_to_elements.
typedef struct {
    HTMLElement* elements;
    int count;
    int cap;
    int ok;
} HTMLConvertResult;

// html_to_elements parses an HTML string and returns an array of structured
// elements. The caller processes these elements to apply terminal styling.
// style/script content is stripped. Block elements get proper spacing.
// Returns a result with ok=1 on success. Caller must free with free_html_result.
HTMLConvertResult html_to_elements(const char* html, size_t len);

// free_html_result frees all memory in an HTMLConvertResult.
void free_html_result(HTMLConvertResult* r);

#endif
