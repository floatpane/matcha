#include "htmlconv.h"
#include <stdlib.h>
#include <string.h>
#include <ctype.h>

// --- Dynamic buffer ---

typedef struct {
    char* data;
    size_t len;
    size_t cap;
} Buffer;

static void buf_init(Buffer* b) {
    b->data = NULL;
    b->len = 0;
    b->cap = 0;
}

static void buf_ensure(Buffer* b, size_t extra) {
    size_t needed = b->len + extra;
    if (needed <= b->cap) return;
    size_t newcap = b->cap ? b->cap * 2 : 256;
    while (newcap < needed) newcap *= 2;
    b->data = (char*)realloc(b->data, newcap);
    b->cap = newcap;
}

static void buf_append(Buffer* b, const char* s, size_t n) {
    if (n == 0) return;
    buf_ensure(b, n);
    memcpy(b->data + b->len, s, n);
    b->len += n;
}

static void buf_append_char(Buffer* b, char c) {
    buf_ensure(b, 1);
    b->data[b->len++] = c;
}

static char* buf_finish(Buffer* b) {
    buf_append_char(b, '\0');
    return b->data;
}

static void buf_free(Buffer* b) {
    free(b->data);
    b->data = NULL;
    b->len = 0;
    b->cap = 0;
}

// --- Result helpers ---

static void result_init(HTMLConvertResult* r) {
    r->elements = NULL;
    r->count = 0;
    r->cap = 0;
    r->ok = 0;
}

static HTMLElement* result_add(HTMLConvertResult* r) {
    if (r->count >= r->cap) {
        int newcap = r->cap ? r->cap * 2 : 32;
        r->elements = (HTMLElement*)realloc(r->elements, sizeof(HTMLElement) * newcap);
        r->cap = newcap;
    }
    HTMLElement* e = &r->elements[r->count++];
    e->type = HELEM_TEXT;
    e->text = NULL;
    e->attr1 = NULL;
    e->attr2 = NULL;
    return e;
}

// Flush accumulated text buffer as a TEXT element.
static void flush_text(HTMLConvertResult* r, Buffer* buf) {
    if (buf->len == 0) return;
    HTMLElement* e = result_add(r);
    e->type = HELEM_TEXT;
    e->text = buf_finish(buf);
    buf_init(buf);
}

// --- HTML entity decoding ---

static size_t decode_entity(const char* s, size_t len, Buffer* out) {
    // s points to '&', returns number of chars consumed
    if (len < 2) { buf_append_char(out, '&'); return 1; }

    // Find the ';'
    size_t end = 1;
    while (end < len && end < 12 && s[end] != ';') end++;
    if (end >= len || s[end] != ';') { buf_append_char(out, '&'); return 1; }

    size_t ent_len = end - 1; // length of entity name (between & and ;)
    const char* name = s + 1;

    // Numeric entities
    if (ent_len >= 2 && name[0] == '#') {
        unsigned long cp = 0;
        if (name[1] == 'x' || name[1] == 'X') {
            for (size_t i = 2; i < ent_len; i++) {
                char c = name[i];
                if (c >= '0' && c <= '9') cp = cp * 16 + (c - '0');
                else if (c >= 'a' && c <= 'f') cp = cp * 16 + 10 + (c - 'a');
                else if (c >= 'A' && c <= 'F') cp = cp * 16 + 10 + (c - 'A');
                else break;
            }
        } else {
            for (size_t i = 1; i < ent_len; i++) {
                if (name[i] >= '0' && name[i] <= '9') cp = cp * 10 + (name[i] - '0');
                else break;
            }
        }
        // Encode as UTF-8
        if (cp < 0x80) {
            buf_append_char(out, (char)cp);
        } else if (cp < 0x800) {
            buf_append_char(out, (char)(0xC0 | (cp >> 6)));
            buf_append_char(out, (char)(0x80 | (cp & 0x3F)));
        } else if (cp < 0x10000) {
            buf_append_char(out, (char)(0xE0 | (cp >> 12)));
            buf_append_char(out, (char)(0x80 | ((cp >> 6) & 0x3F)));
            buf_append_char(out, (char)(0x80 | (cp & 0x3F)));
        } else if (cp < 0x110000) {
            buf_append_char(out, (char)(0xF0 | (cp >> 18)));
            buf_append_char(out, (char)(0x80 | ((cp >> 12) & 0x3F)));
            buf_append_char(out, (char)(0x80 | ((cp >> 6) & 0x3F)));
            buf_append_char(out, (char)(0x80 | (cp & 0x3F)));
        }
        return end + 1;
    }

    // Named entities (common ones)
    struct { const char* name; const char* value; } entities[] = {
        {"lt", "<"}, {"gt", ">"}, {"amp", "&"}, {"quot", "\""},
        {"apos", "'"}, {"nbsp", " "}, {"ndash", "\xe2\x80\x93"},
        {"mdash", "\xe2\x80\x94"}, {"laquo", "\xc2\xab"},
        {"raquo", "\xc2\xbb"}, {"copy", "\xc2\xa9"},
        {"reg", "\xc2\xae"}, {"trade", "\xe2\x84\xa2"},
        {"hellip", "\xe2\x80\xa6"}, {"bull", "\xe2\x80\xa2"},
        {"rsquo", "\xe2\x80\x99"}, {"lsquo", "\xe2\x80\x98"},
        {"rdquo", "\xe2\x80\x9d"}, {"ldquo", "\xe2\x80\x9c"},
        {NULL, NULL}
    };

    for (int i = 0; entities[i].name; i++) {
        if (ent_len == strlen(entities[i].name) &&
            strncmp(name, entities[i].name, ent_len) == 0) {
            buf_append(out, entities[i].value, strlen(entities[i].value));
            return end + 1;
        }
    }

    // Unknown entity - pass through
    buf_append(out, s, end + 1);
    return end + 1;
}

// --- Tag parsing ---

typedef struct {
    char name[64];
    int name_len;
    int is_closing;
    int is_self_closing;
    // Attributes (we parse href, src, alt, cite)
    char href[2048];
    char src[2048];
    char alt[512];
    char cite[2048];
} Tag;

// Case-insensitive compare for tag names.
static int tag_eq(const char* a, int alen, const char* b) {
    int blen = (int)strlen(b);
    if (alen != blen) return 0;
    for (int i = 0; i < alen; i++) {
        if (tolower((unsigned char)a[i]) != tolower((unsigned char)b[i])) return 0;
    }
    return 1;
}

// Parse an attribute value (handles both quoted and unquoted).
// Returns pointer past the parsed value.
static const char* parse_attr_value(const char* p, const char* end, char* out, size_t out_size) {
    if (p >= end) return p;
    char quote = 0;
    if (*p == '"' || *p == '\'') {
        quote = *p++;
    }
    size_t i = 0;
    while (p < end) {
        if (quote) {
            if (*p == quote) { p++; break; }
        } else {
            if (isspace((unsigned char)*p) || *p == '>' || *p == '/') break;
        }
        if (i < out_size - 1) out[i++] = *p;
        p++;
    }
    out[i] = '\0';
    return p;
}

// Parse a tag starting after '<'. Returns pointer past '>'.
static const char* parse_tag(const char* p, const char* end, Tag* tag) {
    memset(tag, 0, sizeof(*tag));

    // Skip whitespace after '<'
    while (p < end && isspace((unsigned char)*p)) p++;

    // Check closing tag
    if (p < end && *p == '/') {
        tag->is_closing = 1;
        p++;
    }

    // Parse tag name
    while (p < end && !isspace((unsigned char)*p) && *p != '>' && *p != '/' &&
           tag->name_len < 63) {
        tag->name[tag->name_len++] = *p++;
    }
    tag->name[tag->name_len] = '\0';

    // Parse attributes
    while (p < end && *p != '>') {
        // Skip whitespace
        while (p < end && isspace((unsigned char)*p)) p++;
        if (p >= end || *p == '>' || *p == '/') break;

        // Parse attribute name
        char attr_name[64] = {0};
        int an = 0;
        while (p < end && *p != '=' && *p != '>' && !isspace((unsigned char)*p) && an < 63) {
            attr_name[an++] = tolower((unsigned char)*p++);
        }
        attr_name[an] = '\0';

        // Skip '='
        while (p < end && isspace((unsigned char)*p)) p++;
        if (p < end && *p == '=') {
            p++;
            while (p < end && isspace((unsigned char)*p)) p++;

            // Parse value into correct field
            if (strcmp(attr_name, "href") == 0) {
                p = parse_attr_value(p, end, tag->href, sizeof(tag->href));
            } else if (strcmp(attr_name, "src") == 0) {
                p = parse_attr_value(p, end, tag->src, sizeof(tag->src));
            } else if (strcmp(attr_name, "alt") == 0) {
                p = parse_attr_value(p, end, tag->alt, sizeof(tag->alt));
            } else if (strcmp(attr_name, "cite") == 0) {
                p = parse_attr_value(p, end, tag->cite, sizeof(tag->cite));
            } else {
                // Skip unknown attribute value
                char discard[4096];
                p = parse_attr_value(p, end, discard, sizeof(discard));
            }
        }
    }

    // Check self-closing and skip past '>'
    if (p < end && *p == '/') {
        tag->is_self_closing = 1;
        p++;
    }
    if (p < end && *p == '>') p++;

    return p;
}

// --- Main parser ---

// Tag stack for nesting tracking.
#define MAX_STACK 128

typedef struct {
    int in_style;      // Inside <style>
    int in_script;     // Inside <script>
    int in_pre;        // Inside <pre>
    int in_a;          // Inside <a>
    char a_href[2048]; // Current link href
    Buffer a_text;     // Current link text accumulator
    int in_h1;
    int in_h2;
    Buffer h_text;     // Current header text accumulator
    int bq_depth;      // Blockquote nesting depth
    Buffer bq_text;    // Current blockquote text accumulator
    char bq_cite[2048];
    Buffer bq_prev;    // Text before blockquote (for "On...wrote:" detection)
    int last_was_block; // Last element was a block (for spacing)
} ParseState;

HTMLConvertResult html_to_elements(const char* html, size_t len) {
    HTMLConvertResult result;
    result_init(&result);

    if (!html || len == 0) {
        result.ok = 1;
        return result;
    }

    ParseState state;
    memset(&state, 0, sizeof(state));
    buf_init(&state.a_text);
    buf_init(&state.h_text);
    buf_init(&state.bq_text);
    buf_init(&state.bq_prev);

    Buffer text_buf;
    buf_init(&text_buf);

    const char* p = html;
    const char* end = html + len;

    while (p < end) {
        if (*p == '<') {
            // Check for comment
            if (p + 3 < end && p[1] == '!' && p[2] == '-' && p[3] == '-') {
                const char* ce = strstr(p + 4, "-->");
                if (ce) { p = ce + 3; continue; }
                p++;
                continue;
            }

            // Check for DOCTYPE/CDATA
            if (p + 1 < end && p[1] == '!') {
                const char* gt = memchr(p, '>', end - p);
                if (gt) { p = gt + 1; continue; }
                p++;
                continue;
            }

            Tag tag;
            const char* after = parse_tag(p + 1, end, &tag);

            // Handle specific tags
            if (tag_eq(tag.name, tag.name_len, "style")) {
                if (tag.is_closing) state.in_style = 0;
                else state.in_style = 1;
                p = after;
                continue;
            }
            if (tag_eq(tag.name, tag.name_len, "script")) {
                if (tag.is_closing) state.in_script = 0;
                else state.in_script = 1;
                p = after;
                continue;
            }

            if (state.in_style || state.in_script) {
                p = after;
                continue;
            }

            // <br> -> newline
            if (tag_eq(tag.name, tag.name_len, "br")) {
                if (state.in_a) {
                    buf_append_char(&state.a_text, '\n');
                } else if (state.in_h1 || state.in_h2) {
                    buf_append_char(&state.h_text, ' ');
                } else if (state.bq_depth > 0) {
                    buf_append_char(&state.bq_text, '\n');
                } else {
                    buf_append_char(&text_buf, '\n');
                }
                p = after;
                continue;
            }

            // <pre>
            if (tag_eq(tag.name, tag.name_len, "pre")) {
                state.in_pre = !tag.is_closing;
                p = after;
                continue;
            }

            // <h1>
            if (tag_eq(tag.name, tag.name_len, "h1")) {
                if (tag.is_closing && state.in_h1) {
                    state.in_h1 = 0;
                    flush_text(&result, &text_buf);
                    HTMLElement* e = result_add(&result);
                    e->type = HELEM_H1;
                    e->text = buf_finish(&state.h_text);
                    buf_init(&state.h_text);
                    // Add block spacing
                    HTMLElement* sp = result_add(&result);
                    sp->type = HELEM_TEXT;
                    sp->text = strdup("\n\n");
                } else if (!tag.is_closing) {
                    flush_text(&result, &text_buf);
                    state.in_h1 = 1;
                    buf_init(&state.h_text);
                }
                p = after;
                continue;
            }

            // <h2>
            if (tag_eq(tag.name, tag.name_len, "h2")) {
                if (tag.is_closing && state.in_h2) {
                    state.in_h2 = 0;
                    flush_text(&result, &text_buf);
                    HTMLElement* e = result_add(&result);
                    e->type = HELEM_H2;
                    e->text = buf_finish(&state.h_text);
                    buf_init(&state.h_text);
                    HTMLElement* sp = result_add(&result);
                    sp->type = HELEM_TEXT;
                    sp->text = strdup("\n\n");
                } else if (!tag.is_closing) {
                    flush_text(&result, &text_buf);
                    state.in_h2 = 1;
                    buf_init(&state.h_text);
                }
                p = after;
                continue;
            }

            // <a>
            if (tag_eq(tag.name, tag.name_len, "a")) {
                if (tag.is_closing && state.in_a) {
                    state.in_a = 0;
                    // If inside blockquote, emit link text inline
                    if (state.bq_depth > 0) {
                        if (state.a_text.len > 0) {
                            buf_append(&state.bq_text, state.a_text.data, state.a_text.len);
                        }
                        buf_free(&state.a_text);
                    } else {
                        flush_text(&result, &text_buf);
                        HTMLElement* e = result_add(&result);
                        e->type = HELEM_LINK;
                        e->text = buf_finish(&state.a_text);
                        e->attr1 = strdup(state.a_href);
                        buf_init(&state.a_text);
                    }
                } else if (!tag.is_closing && tag.href[0]) {
                    if (state.bq_depth == 0) flush_text(&result, &text_buf);
                    state.in_a = 1;
                    strncpy(state.a_href, tag.href, sizeof(state.a_href) - 1);
                    state.a_href[sizeof(state.a_href) - 1] = '\0';
                    buf_init(&state.a_text);
                }
                p = after;
                continue;
            }

            // <img>
            if (tag_eq(tag.name, tag.name_len, "img")) {
                if (tag.src[0]) {
                    flush_text(&result, &text_buf);
                    HTMLElement* e = result_add(&result);
                    e->type = HELEM_IMAGE;
                    e->attr1 = strdup(tag.src);
                    e->attr2 = tag.alt[0] ? strdup(tag.alt) : strdup("Does not contain alt text");
                }
                p = after;
                continue;
            }

            // <blockquote>
            if (tag_eq(tag.name, tag.name_len, "blockquote")) {
                if (tag.is_closing && state.bq_depth > 0) {
                    state.bq_depth--;
                    if (state.bq_depth == 0) {
                        flush_text(&result, &text_buf);
                        HTMLElement* e = result_add(&result);
                        e->type = HELEM_BLOCKQUOTE;
                        e->text = buf_finish(&state.bq_text);
                        if (tag.cite[0]) {
                            e->attr1 = strdup(tag.cite);
                        }
                        if (state.bq_prev.len > 0) {
                            e->attr2 = buf_finish(&state.bq_prev);
                        }
                        buf_init(&state.bq_text);
                        buf_init(&state.bq_prev);
                    }
                } else if (!tag.is_closing) {
                    if (state.bq_depth == 0) {
                        // Capture preceding text for "On...wrote:" detection
                        // Look back in text_buf for the last line
                        buf_free(&state.bq_prev);
                        buf_init(&state.bq_prev);
                        if (text_buf.len > 0) {
                            // Find last non-empty line
                            int start = (int)text_buf.len - 1;
                            while (start > 0 && text_buf.data[start] == '\n') start--;
                            int line_start = start;
                            while (line_start > 0 && text_buf.data[line_start - 1] != '\n') line_start--;
                            int line_len = start - line_start + 1;
                            if (line_len > 0) {
                                buf_append(&state.bq_prev, text_buf.data + line_start, line_len);
                            }
                        }
                        flush_text(&result, &text_buf);
                        buf_init(&state.bq_text);
                    }
                    if (tag.cite[0]) {
                        strncpy(state.bq_cite, tag.cite, sizeof(state.bq_cite) - 1);
                    }
                    state.bq_depth++;
                }
                p = after;
                continue;
            }

            // Block elements: add spacing
            if (tag_eq(tag.name, tag.name_len, "p") ||
                tag_eq(tag.name, tag.name_len, "div") ||
                tag_eq(tag.name, tag.name_len, "li") ||
                tag_eq(tag.name, tag.name_len, "tr") ||
                tag_eq(tag.name, tag.name_len, "table") ||
                tag_eq(tag.name, tag.name_len, "hr")) {
                if (tag.is_closing || tag_eq(tag.name, tag.name_len, "hr")) {
                    if (state.bq_depth > 0) {
                        buf_append(&state.bq_text, "\n\n", 2);
                    } else {
                        buf_append(&text_buf, "\n\n", 2);
                    }
                    state.last_was_block = 1;
                }
                p = after;
                continue;
            }

            // <ul>, <ol>, <dl>, <thead>, <tbody>, etc. - skip tag but process children
            p = after;
            continue;
        }

        // Text content
        if (state.in_style || state.in_script) {
            p++;
            continue;
        }

        // Handle entities
        if (*p == '&') {
            Buffer* target;
            if (state.in_a) target = &state.a_text;
            else if (state.in_h1 || state.in_h2) target = &state.h_text;
            else if (state.bq_depth > 0) target = &state.bq_text;
            else target = &text_buf;

            size_t consumed = decode_entity(p, end - p, target);
            p += consumed;
            continue;
        }

        // Regular character
        char c = *p++;
        if (state.in_a) {
            buf_append_char(&state.a_text, c);
        } else if (state.in_h1 || state.in_h2) {
            buf_append_char(&state.h_text, c);
        } else if (state.bq_depth > 0) {
            buf_append_char(&state.bq_text, c);
        } else {
            buf_append_char(&text_buf, c);
        }
    }

    // Flush remaining text
    flush_text(&result, &text_buf);

    // Flush any unclosed elements
    if (state.in_h1 || state.in_h2) {
        HTMLElement* e = result_add(&result);
        e->type = state.in_h1 ? HELEM_H1 : HELEM_H2;
        e->text = buf_finish(&state.h_text);
    } else {
        buf_free(&state.h_text);
    }

    if (state.in_a) {
        HTMLElement* e = result_add(&result);
        e->type = HELEM_LINK;
        e->text = buf_finish(&state.a_text);
        e->attr1 = strdup(state.a_href);
    } else {
        buf_free(&state.a_text);
    }

    if (state.bq_depth > 0) {
        HTMLElement* e = result_add(&result);
        e->type = HELEM_BLOCKQUOTE;
        e->text = buf_finish(&state.bq_text);
        if (state.bq_prev.len > 0) {
            e->attr2 = buf_finish(&state.bq_prev);
        }
    } else {
        buf_free(&state.bq_text);
        buf_free(&state.bq_prev);
    }

    result.ok = 1;
    return result;
}

void free_html_result(HTMLConvertResult* r) {
    if (!r) return;
    for (int i = 0; i < r->count; i++) {
        free(r->elements[i].text);
        free(r->elements[i].attr1);
        free(r->elements[i].attr2);
    }
    free(r->elements);
    r->elements = NULL;
    r->count = 0;
    r->cap = 0;
}
