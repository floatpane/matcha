#include "base64wrap.h"
#include <stdlib.h>
#include <string.h>

char* wrap_base64(const char* data, size_t len, size_t* out_len) {
    if (len == 0) {
        char* empty = (char*)malloc(1);
        if (!empty) return NULL;
        empty[0] = '\0';
        *out_len = 0;
        return empty;
    }

    const size_t line_len = 76;
    // Number of full lines (each gets \r\n appended except the last)
    size_t num_breaks = (len > line_len) ? (len - 1) / line_len : 0;
    size_t result_len = len + num_breaks * 2; // each break adds \r\n

    char* result = (char*)malloc(result_len + 1);
    if (!result) return NULL;

    size_t src_pos = 0;
    size_t dst_pos = 0;

    while (src_pos < len) {
        size_t chunk = len - src_pos;
        if (chunk > line_len) chunk = line_len;

        memcpy(result + dst_pos, data + src_pos, chunk);
        dst_pos += chunk;
        src_pos += chunk;

        if (src_pos < len) {
            result[dst_pos++] = '\r';
            result[dst_pos++] = '\n';
        }
    }

    result[dst_pos] = '\0';
    *out_len = dst_pos;
    return result;
}
