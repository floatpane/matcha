#ifndef MATCHA_BASE64WRAP_H
#define MATCHA_BASE64WRAP_H

#include <stddef.h>

// wrap_base64 wraps base64-encoded data at 76 characters per line with \r\n
// separators, as required by MIME (RFC 2045). The caller must free the returned
// pointer with free().
// Returns NULL if allocation fails.
char* wrap_base64(const char* data, size_t len, size_t* out_len);

#endif
