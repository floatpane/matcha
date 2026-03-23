#ifndef MD4C_HTML_H
#define MD4C_HTML_H

#include "md4c.h"

#ifdef __cplusplus
    extern "C" {
#endif

#define MD_HTML_FLAG_DEBUG                  0x0001
#define MD_HTML_FLAG_VERBATIM_ENTITIES      0x0002
#define MD_HTML_FLAG_SKIP_UTF8_BOM          0x0004
#define MD_HTML_FLAG_XHTML                  0x0008

int md_html(const MD_CHAR* input, MD_SIZE input_size,
            void (*process_output)(const MD_CHAR*, MD_SIZE, void*),
            void* userdata, unsigned parser_flags, unsigned renderer_flags);

#ifdef __cplusplus
    }
#endif

#endif
