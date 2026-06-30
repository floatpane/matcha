#ifndef MATCHA_HTMLCONV_H
#define MATCHA_HTMLCONV_H

#include <stddef.h>

enum {
    HELEM_TEXT       = 0,
    HELEM_H1         = 1,
    HELEM_H2         = 2,
    HELEM_LINK       = 3,
    HELEM_IMAGE      = 4,
    HELEM_BLOCKQUOTE = 5,
    HELEM_TABLE      = 6,
    HELEM_CODE       = 7,
    HELEM_H3         = 8,
    HELEM_H4         = 9,
    HELEM_H5         = 10,
    HELEM_H6         = 11,
    HELEM_LIST_ITEM  = 12,
    HELEM_HR         = 13,
};

#define HTML_STYLE_BOLD          0x01
#define HTML_STYLE_ITALIC        0x02
#define HTML_STYLE_UNDERLINE     0x04
#define HTML_STYLE_STRIKETHROUGH 0x08

typedef struct {
    int type;
    int style;
    char* text;
    char* attr1;
    char* attr2;
} HTMLElement;

typedef struct {
    HTMLElement* elements;
    int count;
    int cap;
    int ok;
} HTMLConvertResult;

HTMLConvertResult html_to_elements(const char* html, size_t len);

void free_html_result(HTMLConvertResult* r);

#endif
