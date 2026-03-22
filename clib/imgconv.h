#ifndef MATCHA_IMGCONV_H
#define MATCHA_IMGCONV_H

#include <stddef.h>

// ImageResult holds the output of decode_to_png.
typedef struct {
    unsigned char* png_data; // PNG-encoded bytes (caller must free)
    size_t png_len;          // Length of png_data
    int width;               // Image width in pixels
    int height;              // Image height in pixels
    int ok;                  // 1 on success, 0 on failure
} ImageResult;

// decode_to_png takes raw image bytes (JPEG, PNG, BMP, GIF, etc.),
// decodes them, and re-encodes as PNG. Supports any format that
// stb_image can decode.
ImageResult decode_to_png(const unsigned char* data, size_t len);

// free_image_result frees the PNG data inside an ImageResult.
void free_image_result(ImageResult* r);

// image_dimensions returns only the width and height of an image without
// fully decoding pixel data. Faster than decode_to_png when you only need
// dimensions. Returns 1 on success, 0 on failure.
int image_dimensions(const unsigned char* data, size_t len, int* width, int* height);

#endif
