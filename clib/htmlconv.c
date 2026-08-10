#include "htmlconv.h"
#include <stdlib.h>
#include <string.h>
#include <ctype.h>
#include <stdio.h>

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

// Append a text character with HTML whitespace collapsing.
// In non-pre mode, collapses runs of whitespace to a single space.
static void buf_append_html_char(Buffer* b, char c, int in_pre) {
    if (in_pre) {
        buf_append_char(b, c);
        return;
    }
    if (c == '\n' || c == '\r' || c == '\t') c = ' ';
    if (c == ' ') {
        // Skip if buffer already ends with space or is empty
        if (b->len == 0 || b->data[b->len - 1] == ' ' || b->data[b->len - 1] == '\n') return;
    }
    buf_append_char(b, c);
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
    e->style = 0;
    e->text = NULL;
    e->attr1 = NULL;
    e->attr2 = NULL;
    return e;
}

// Flush accumulated text buffer as a TEXT element.
static void flush_text(HTMLConvertResult* r, Buffer* buf, int style) {
    if (buf->len == 0) return;
    HTMLElement* e = result_add(r);
    e->type = HELEM_TEXT;
    e->style = style;
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
    // Attributes (we parse href, src, alt, cite, class)
    char href[2048];
    char src[2048];
    char alt[512];
    char cite[2048];
    char klass[256];
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
            } else if (strcmp(attr_name, "class") == 0) {
                p = parse_attr_value(p, end, tag->klass, sizeof(tag->klass));
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

// extract_language parses a class attribute like "language-go" or
// "prettyprint language-python" and writes the language token (e.g. "go")
// into out. out_size is the capacity of out including the NUL terminator.
static void extract_language(const char* klass, char* out, size_t out_size) {
    out[0] = '\0';
    if (!klass || klass[0] == '\0') return;
    const char* p = klass;
    while (*p) {
        // Skip non-matching tokens
        const char* tok = p;
        while (*p && *p != ' ') p++;
        size_t tok_len = (size_t)(p - tok);
        const char* prefix = "language-";
        size_t prefix_len = strlen(prefix);
        if (tok_len > prefix_len && strncmp(tok, prefix, prefix_len) == 0) {
            size_t lang_len = tok_len - prefix_len;
            if (lang_len >= out_size) lang_len = out_size - 1;
            memcpy(out, tok + prefix_len, lang_len);
            out[lang_len] = '\0';
            return;
        }
        while (*p == ' ') p++;
    }
}

// Tag stack for nesting tracking.
#define MAX_STACK 128
#define MAX_LIST_DEPTH 16

typedef struct {
    int in_style;      // Inside <style>
    int in_script;     // Inside <script>
    int in_pre;        // Inside <pre>
    int pre_depth;     // <pre> nesting depth (normally 0 or 1)
    Buffer pre_buf;    // Captured <pre> content for HELEM_CODE
    char pre_lang[128]; // Language hint extracted from class="language-XXX"
    int in_a;          // Inside <a>
    char a_href[2048]; // Current link href
    Buffer a_text;     // Current link text accumulator
    int in_h1;
    int in_h2;
    int h_level;       // Active heading level (0 = none, 1-6)
    Buffer h_text;     // Current header text accumulator
    int bq_depth;      // Blockquote nesting depth
    Buffer bq_text;    // Current blockquote text accumulator
    char bq_cite[2048];
    Buffer bq_prev;    // Text before blockquote (for "On...wrote:" detection)
    int last_was_block; // Last element was a block (for spacing)
    // Table state
    int table_depth;    // Table nesting depth (0 = not in any table)
    int capture_depth;  // Depth at which we're capturing data table (-1 = not capturing)
    int in_thead;       // Inside <thead> (at capture depth)
    int in_tr;          // Inside <tr> (at capture depth, capturing mode)
    int in_td;          // Inside <td>/<th> (at capture depth, capturing mode)
    int cell_index;     // Cell index within current row
    int row_index;      // Row index within current table
    int header_rows;    // Number of header rows (rows inside <thead>)
    Buffer cell_text;   // Current cell text accumulator
    Buffer table_data;  // Accumulated table data (cells tab-separated, rows newline-separated)
    // List state
    int list_depth;                       // Current nesting depth (0 = not in a list)
    int list_ordered[MAX_LIST_DEPTH];     // 1 if <ol>, 0 if <ul>, at each depth
    int list_index[MAX_LIST_DEPTH];       // 1-based item index per depth
    int in_li;                            // Currently inside a <li>
    Buffer li_buf;                        // Current list item text accumulator
    int pending_space;                    // Keep next leading space after a span flush
    // Inline text style stack (bitmask of HTML_STYLE_*)
    int style_stack[MAX_STACK];
    int style_depth;
    int cur_style;                        // Current combined style mask
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
    state.capture_depth = -1;  // Not capturing
    buf_init(&state.a_text);
    buf_init(&state.h_text);
    buf_init(&state.bq_text);
    buf_init(&state.bq_prev);
    buf_init(&state.cell_text);
    buf_init(&state.table_data);
    buf_init(&state.pre_buf);
    buf_init(&state.li_buf);

    Buffer text_buf;
    buf_init(&text_buf);

    const char* p = html;
    const char* end = html + len;

    while (p < end) {
        if (*p == '<') {
            state.pending_space = 0;
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
                if (state.pre_depth > 0) {
                    buf_append_char(&state.pre_buf, '\n');
                } else if (state.in_td) {
                    buf_append_char(&state.cell_text, ' ');
                } else if (state.in_a) {
                    buf_append_char(&state.a_text, '\n');
                } else if (state.h_level > 0) {
                    buf_append_char(&state.h_text, ' ');
                } else if (state.bq_depth > 0) {
                    buf_append_char(&state.bq_text, '\n');
                } else if (state.in_li) {
                    buf_append_char(&state.li_buf, '\n');
                } else {
                    buf_append_char(&text_buf, '\n');
                }
                p = after;
                continue;
            }

            // <pre> — capture into pre_buf and emit HELEM_CODE on close.
            if (tag_eq(tag.name, tag.name_len, "pre")) {
                if (!tag.is_closing) {
                    if (state.pre_depth == 0) {
                        flush_text(&result, &text_buf, state.cur_style);
                        buf_free(&state.pre_buf);
                        buf_init(&state.pre_buf);
                        state.pre_lang[0] = '\0';
                        // md4c emits <pre> without class; language is on <code>.
                        // Still check for class here for hand-written HTML.
                        extract_language(tag.klass, state.pre_lang, sizeof(state.pre_lang));
                    }
                    state.pre_depth++;
                    state.in_pre = 1;
                } else if (state.pre_depth > 0) {
                    state.pre_depth--;
                    if (state.pre_depth == 0) {
                        state.in_pre = 0;
                        flush_text(&result, &text_buf, state.cur_style);
                        HTMLElement* e = result_add(&result);
                        e->type = HELEM_CODE;
                        e->text = buf_finish(&state.pre_buf);
                        buf_init(&state.pre_buf);
                        if (state.pre_lang[0]) {
                            e->attr1 = strdup(state.pre_lang);
                        }
                        state.pre_lang[0] = '\0';
                        // Add block spacing after the code block
                        HTMLElement* sp = result_add(&result);
                        sp->type = HELEM_TEXT;
                        sp->text = strdup("\n\n");
                    }
                }
                p = after;
                continue;
            }

            // <code> inside <pre> — capture language hint from class attribute.
            // md4c emits <pre><code class="language-XXX">; the opening <pre> has
            // no class, so we grab the language when <code> opens.
            if (tag_eq(tag.name, tag.name_len, "code") && !tag.is_closing &&
                state.pre_depth > 0 && state.pre_lang[0] == '\0') {
                extract_language(tag.klass, state.pre_lang, sizeof(state.pre_lang));
                p = after;
                continue;
            }

            // <h1> - <h6>
            if (tag.name_len == 2 && (tag.name[0] == 'h' || tag.name[0] == 'H') &&
                tag.name[1] >= '1' && tag.name[1] <= '6') {
                int level = tag.name[1] - '0';
                if (tag.is_closing && state.h_level == level) {
                    state.h_level = 0;
                    if (level == 1) state.in_h1 = 0;
                    if (level == 2) state.in_h2 = 0;
                    flush_text(&result, &text_buf, state.cur_style);
                    HTMLElement* e = result_add(&result);
                    switch (level) {
                        case 1: e->type = HELEM_H1; break;
                        case 2: e->type = HELEM_H2; break;
                        case 3: e->type = HELEM_H3; break;
                        case 4: e->type = HELEM_H4; break;
                        case 5: e->type = HELEM_H5; break;
                        case 6: e->type = HELEM_H6; break;
                    }
                    e->text = buf_finish(&state.h_text);
                    buf_init(&state.h_text);
                    HTMLElement* sp = result_add(&result);
                    sp->type = HELEM_TEXT;
                    sp->text = strdup("\n\n");
                } else if (!tag.is_closing) {
                    flush_text(&result, &text_buf, state.cur_style);
                    state.h_level = level;
                    if (level == 1) state.in_h1 = 1;
                    if (level == 2) state.in_h2 = 1;
                    buf_init(&state.h_text);
                }
                p = after;
                continue;
            }

            // <a>
            if (tag_eq(tag.name, tag.name_len, "a")) {
                if (tag.is_closing && state.in_a) {
                    state.in_a = 0;
                    // If inside blockquote or list item, emit link text inline
                    if (state.bq_depth > 0) {
                        if (state.a_text.len > 0) {
                            buf_append(&state.bq_text, state.a_text.data, state.a_text.len);
                        }
                        buf_free(&state.a_text);
                    } else if (state.in_li) {
                        if (state.a_text.len > 0) {
                            buf_append(&state.li_buf, state.a_text.data, state.a_text.len);
                        }
                        buf_free(&state.a_text);
                    } else {
                        flush_text(&result, &text_buf, state.cur_style);
                        HTMLElement* e = result_add(&result);
                        e->type = HELEM_LINK;
                        e->text = buf_finish(&state.a_text);
                        e->attr1 = strdup(state.a_href);
                        buf_init(&state.a_text);
                    }
                } else if (!tag.is_closing && tag.href[0]) {
                    if (state.bq_depth == 0 && !state.in_li) flush_text(&result, &text_buf, state.cur_style);
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
                    flush_text(&result, &text_buf, state.cur_style);
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
                        flush_text(&result, &text_buf, state.cur_style);
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
                        flush_text(&result, &text_buf, state.cur_style);
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

            // <table>
            if (tag_eq(tag.name, tag.name_len, "table")) {
                if (!tag.is_closing) {
                    state.table_depth++;
                } else if (state.table_depth > 0) {
                    if (state.table_depth == state.capture_depth) {
                        // Closing the data table we're capturing
                        if (state.table_data.len > 0) {
                            flush_text(&result, &text_buf, state.cur_style);
                            HTMLElement* e = result_add(&result);
                            e->type = HELEM_TABLE;
                            e->text = buf_finish(&state.table_data);
                            buf_init(&state.table_data);
                            char hdr_buf[16];
                            snprintf(hdr_buf, sizeof(hdr_buf), "%d", state.header_rows);
                            e->attr1 = strdup(hdr_buf);
                        } else {
                            buf_free(&state.table_data);
                            buf_init(&state.table_data);
                        }
                        state.capture_depth = -1;
                        state.in_td = 0;
                        state.in_tr = 0;
                    } else {
                        // Layout or nested table: block spacing
                        if (state.bq_depth > 0) {
                            buf_append(&state.bq_text, "\n\n", 2);
                        } else {
                            buf_append(&text_buf, "\n\n", 2);
                        }
                        state.last_was_block = 1;
                    }
                    state.table_depth--;
                }
                p = after;
                continue;
            }

            // <thead>
            if (tag_eq(tag.name, tag.name_len, "thead")) {
                if (state.table_depth == state.capture_depth) {
                    state.in_thead = !tag.is_closing;
                }
                p = after;
                continue;
            }

            // <tbody>, <tfoot> - skip tag
            if (tag_eq(tag.name, tag.name_len, "tbody") ||
                tag_eq(tag.name, tag.name_len, "tfoot")) {
                p = after;
                continue;
            }

            // <tr>
            if (tag_eq(tag.name, tag.name_len, "tr")) {
                if (state.table_depth == state.capture_depth) {
                    // Data table row
                    if (!tag.is_closing) {
                        if (state.row_index > 0) {
                            buf_append_char(&state.table_data, '\n');
                        }
                        state.in_tr = 1;
                        state.cell_index = 0;
                    } else {
                        state.in_tr = 0;
                        if (state.in_thead) state.header_rows++;
                        state.row_index++;
                    }
                } else if (tag.is_closing) {
                    // Layout table: block spacing
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

            // <td>, <th>
            if (tag_eq(tag.name, tag.name_len, "td") ||
                tag_eq(tag.name, tag.name_len, "th")) {
                // <th> enables capture mode for the current table depth
                if (tag_eq(tag.name, tag.name_len, "th") && !tag.is_closing &&
                    state.capture_depth < 0 && state.table_depth > 0) {
                    state.capture_depth = state.table_depth;
                    // Start tracking the current row
                    state.in_tr = 1;
                    state.cell_index = 0;
                    state.row_index = 0;
                    state.header_rows = 0;
                    state.in_thead = 1; // <th> implies header
                    buf_free(&state.table_data);
                    buf_init(&state.table_data);
                }

                if (state.table_depth == state.capture_depth && state.in_tr) {
                    if (!tag.is_closing) {
                        state.in_td = 1;
                        buf_free(&state.cell_text);
                        buf_init(&state.cell_text);
                    } else if (state.in_td) {
                        state.in_td = 0;
                        // Append cell to row (tab-separated)
                        if (state.cell_index > 0) {
                            buf_append_char(&state.table_data, '\t');
                        }
                        if (state.cell_text.len > 0) {
                            buf_append(&state.table_data, state.cell_text.data, state.cell_text.len);
                        }
                        buf_free(&state.cell_text);
                        buf_init(&state.cell_text);
                        state.cell_index++;
                    }
                }
                // For layout tables (not capturing at this depth): td/th tags are
                // ignored, text content flows through to text_buf naturally
                p = after;
                continue;
            }

            // <hr> — emit a horizontal rule element
            if (tag_eq(tag.name, tag.name_len, "hr")) {
                flush_text(&result, &text_buf, state.cur_style);
                HTMLElement* e = result_add(&result);
                e->type = HELEM_HR;
                e->text = strdup("");
                HTMLElement* sp = result_add(&result);
                sp->type = HELEM_TEXT;
                sp->text = strdup("\n\n");
                p = after;
                continue;
            }

            // <ul>, <ol>
            if (tag_eq(tag.name, tag.name_len, "ul") ||
                tag_eq(tag.name, tag.name_len, "ol")) {
                if (!tag.is_closing) {
                    if (state.in_li && state.li_buf.len > 0) {
                        flush_text(&result, &text_buf, state.cur_style);
                        HTMLElement* e = result_add(&result);
                        e->type = HELEM_LIST_ITEM;
                        e->text = buf_finish(&state.li_buf);
                        buf_init(&state.li_buf);
                        char depth_buf[16];
                        int outer_depth = state.list_depth - 1;
                        if (outer_depth < 0) outer_depth = 0;
                        snprintf(depth_buf, sizeof(depth_buf), "%d", outer_depth);
                        e->attr1 = strdup(depth_buf);
                        if (state.list_depth > 0 &&
                            state.list_ordered[state.list_depth - 1]) {
                            char idx_buf[16];
                            snprintf(idx_buf, sizeof(idx_buf), "%d",
                                     state.list_index[state.list_depth - 1]);
                            e->attr2 = strdup(idx_buf);
                            state.list_index[state.list_depth - 1]++;
                        }
                    }
                    if (state.list_depth < MAX_LIST_DEPTH) {
                        state.list_depth++;
                        state.list_ordered[state.list_depth - 1] =
                            tag_eq(tag.name, tag.name_len, "ol") ? 1 : 0;
                        state.list_index[state.list_depth - 1] = 1;
                    }
                    if (state.bq_depth == 0 && !state.in_li) flush_text(&result, &text_buf, state.cur_style);
                } else if (state.list_depth > 0) {
                    state.list_depth--;
                    if (state.list_depth == 0) {
                        HTMLElement* sp = result_add(&result);
                        sp->type = HELEM_TEXT;
                        sp->text = strdup("\n\n");
                    }
                }
                p = after;
                continue;
            }

            // <li>
            if (tag_eq(tag.name, tag.name_len, "li")) {
                if (!tag.is_closing) {
                    if (state.list_depth > 0) {
                        flush_text(&result, &text_buf, state.cur_style);
                        buf_free(&state.li_buf);
                        buf_init(&state.li_buf);
                        state.in_li = 1;
                    } else {
                        flush_text(&result, &text_buf, state.cur_style);
                    }
                } else if (state.in_li) {
                    state.in_li = 0;
                    flush_text(&result, &text_buf, state.cur_style);
                    HTMLElement* e = result_add(&result);
                    e->type = HELEM_LIST_ITEM;
                    e->text = buf_finish(&state.li_buf);
                    buf_init(&state.li_buf);
                    char depth_buf[16];
                    snprintf(depth_buf, sizeof(depth_buf), "%d", state.list_depth - 1);
                    e->attr1 = strdup(depth_buf);
                    if (state.list_depth > 0 &&
                        state.list_ordered[state.list_depth - 1]) {
                        char idx_buf[16];
                        snprintf(idx_buf, sizeof(idx_buf), "%d",
                                 state.list_index[state.list_depth - 1]);
                        e->attr2 = strdup(idx_buf);
                        state.list_index[state.list_depth - 1]++;
                    }
                }
                p = after;
                continue;
            }

            // Inline text formatting: <b>, <strong>, <i>, <em>, <u>, <s>, <del>
            if (tag_eq(tag.name, tag.name_len, "b") ||
                tag_eq(tag.name, tag.name_len, "strong") ||
                tag_eq(tag.name, tag.name_len, "i") ||
                tag_eq(tag.name, tag.name_len, "em") ||
                tag_eq(tag.name, tag.name_len, "u") ||
                tag_eq(tag.name, tag.name_len, "s") ||
                tag_eq(tag.name, tag.name_len, "del")) {
                int mask = 0;
                if (tag_eq(tag.name, tag.name_len, "b") ||
                    tag_eq(tag.name, tag.name_len, "strong"))
                    mask = HTML_STYLE_BOLD;
                else if (tag_eq(tag.name, tag.name_len, "i") ||
                         tag_eq(tag.name, tag.name_len, "em"))
                    mask = HTML_STYLE_ITALIC;
                else if (tag_eq(tag.name, tag.name_len, "u"))
                    mask = HTML_STYLE_UNDERLINE;
                else
                    mask = HTML_STYLE_STRIKETHROUGH;
                // Flush pending text with the current style before it changes,
                // so each styled span is emitted as its own TEXT element.
                // Only when the main text_buf is the active target.
                if (state.pre_depth == 0 && !state.in_td && !state.in_a &&
                    state.h_level == 0 && state.bq_depth == 0 && !state.in_li) {
                    if (text_buf.len > 0) {
                        char last = text_buf.data[text_buf.len - 1];
                        if (last != ' ' && last != '\n')
                            state.pending_space = 1;
                    }
                    flush_text(&result, &text_buf, state.cur_style);
                }
                if (!tag.is_closing) {
                    if (state.style_depth < MAX_STACK) {
                        state.style_stack[state.style_depth++] = mask;
                        state.cur_style |= mask;
                    }
                } else if (state.style_depth > 0) {
                    state.style_depth--;
                    int old = state.style_stack[state.style_depth];
                    state.cur_style &= ~old;
                }
                p = after;
                continue;
            }

            // Block elements: add spacing
            if (tag_eq(tag.name, tag.name_len, "p") ||
                tag_eq(tag.name, tag.name_len, "div")) {
                if (tag.is_closing) {
                    if (state.pre_depth > 0) {
                        buf_append_char(&state.pre_buf, '\n');
                    } else if (state.in_li) {
                        buf_append_char(&state.li_buf, '\n');
                    } else if (state.bq_depth > 0) {
                        buf_append(&state.bq_text, "\n\n", 2);
                    } else if (state.in_td) {
                        buf_append_char(&state.cell_text, ' ');
                    } else {
                        buf_append(&text_buf, "\n\n", 2);
                    }
                    state.last_was_block = 1;
                }
                p = after;
                continue;
            }

            // <dl>, etc. - skip tag but process children
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
            if (state.pre_depth > 0) target = &state.pre_buf;
            else if (state.in_td) target = &state.cell_text;
            else if (state.in_a) target = &state.a_text;
            else if (state.h_level > 0) target = &state.h_text;
            else if (state.in_li) target = &state.li_buf;
            else if (state.bq_depth > 0) target = &state.bq_text;
            else target = &text_buf;

            size_t consumed = decode_entity(p, end - p, target);
            p += consumed;
            continue;
        }

        // Regular character — collapse whitespace like HTML (unless in <pre>)
        char c = *p++;
        // Preserve a single leading space that followed a styled span flush.
        if (state.pending_space) {
            state.pending_space = 0;
            if ((c == ' ' || c == '\n' || c == '\r' || c == '\t') &&
                state.pre_depth == 0 && !state.in_td && !state.in_a &&
                state.h_level == 0 && state.bq_depth == 0 && !state.in_li &&
                text_buf.len == 0) {
                buf_append_char(&text_buf, ' ');
                if (c == ' ' || c == '\n' || c == '\r' || c == '\t') {
                    continue;
                }
            }
        }
        if (state.pre_depth > 0) {
            buf_append_char(&state.pre_buf, c);
        } else if (state.in_td) {
            buf_append_html_char(&state.cell_text, c, state.in_pre);
        } else if (state.in_a) {
            buf_append_html_char(&state.a_text, c, state.in_pre);
        } else if (state.h_level > 0) {
            buf_append_html_char(&state.h_text, c, state.in_pre);
        } else if (state.in_li) {
            buf_append_html_char(&state.li_buf, c, state.in_pre);
        } else if (state.bq_depth > 0) {
            buf_append_html_char(&state.bq_text, c, state.in_pre);
        } else {
            buf_append_html_char(&text_buf, c, state.in_pre);
        }
    }

    // Flush remaining text
    flush_text(&result, &text_buf, state.cur_style);

    // Flush any unclosed elements
    if (state.h_level > 0) {
        HTMLElement* e = result_add(&result);
        switch (state.h_level) {
            case 1: e->type = HELEM_H1; break;
            case 2: e->type = HELEM_H2; break;
            case 3: e->type = HELEM_H3; break;
            case 4: e->type = HELEM_H4; break;
            case 5: e->type = HELEM_H5; break;
            case 6: e->type = HELEM_H6; break;
            default: e->type = HELEM_H6; break;
        }
        e->text = buf_finish(&state.h_text);
    } else {
        buf_free(&state.h_text);
    }

    if (state.in_li) {
        state.in_li = 0;
        HTMLElement* e = result_add(&result);
        e->type = HELEM_LIST_ITEM;
        e->text = buf_finish(&state.li_buf);
        char depth_buf[16];
        snprintf(depth_buf, sizeof(depth_buf), "%d", state.list_depth - 1);
        e->attr1 = strdup(depth_buf);
        if (state.list_depth > 0 &&
            state.list_ordered[state.list_depth - 1]) {
            char idx_buf[16];
            snprintf(idx_buf, sizeof(idx_buf), "%d",
                     state.list_index[state.list_depth - 1]);
            e->attr2 = strdup(idx_buf);
        }
    } else {
        buf_free(&state.li_buf);
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

    // Flush unclosed table
    if (state.capture_depth > 0 && state.table_data.len > 0) {
        HTMLElement* e = result_add(&result);
        e->type = HELEM_TABLE;
        e->text = buf_finish(&state.table_data);
        char hdr_buf[16];
        snprintf(hdr_buf, sizeof(hdr_buf), "%d", state.header_rows);
        e->attr1 = strdup(hdr_buf);
    } else {
        buf_free(&state.table_data);
    }
    buf_free(&state.cell_text);

    // Flush unclosed <pre> as a code block
    if (state.pre_depth > 0 && state.pre_buf.len > 0) {
        HTMLElement* e = result_add(&result);
        e->type = HELEM_CODE;
        e->text = buf_finish(&state.pre_buf);
        if (state.pre_lang[0]) {
            e->attr1 = strdup(state.pre_lang);
        }
    } else {
        buf_free(&state.pre_buf);
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
