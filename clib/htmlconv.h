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
};

typedef struct {
    int type;
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
