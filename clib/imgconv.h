#ifndef MATCHA_IMGCONV_H
#define MATCHA_IMGCONV_H

#include <stddef.h>

typedef struct {
    unsigned char* png_data;
    size_t png_len;
    int width;
    int height;
    int ok;
} ImageResult;

ImageResult decode_to_png(const unsigned char* data, size_t len);

void free_image_result(ImageResult* r);

int image_dimensions(const unsigned char* data, size_t len, int* width, int* height);

#endif
