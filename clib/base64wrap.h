#ifndef MATCHA_BASE64WRAP_H
#define MATCHA_BASE64WRAP_H

#include <stddef.h>

char* wrap_base64(const char* data, size_t len, size_t* out_len);

#endif
