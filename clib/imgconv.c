#define STB_IMAGE_IMPLEMENTATION
#include "stb_image.h"

#define STB_IMAGE_WRITE_IMPLEMENTATION
#include "stb_image_write.h"

#include "imgconv.h"
#include <stdlib.h>
#include <string.h>

// write callback for stb_image_write_to_func — appends to a dynamic buffer.
typedef struct {
    unsigned char* buf;
    size_t len;
    size_t cap;
} PngBuffer;

static void png_write_cb(void* context, void* data, int size) {
    PngBuffer* pb = (PngBuffer*)context;
    if (size <= 0) return;

    size_t needed = pb->len + (size_t)size;
    if (needed > pb->cap) {
        size_t new_cap = pb->cap * 2;
        if (new_cap < needed) new_cap = needed;
        unsigned char* new_buf = (unsigned char*)realloc(pb->buf, new_cap);
        if (!new_buf) return; // allocation failure, partial write
        pb->buf = new_buf;
        pb->cap = new_cap;
    }
    memcpy(pb->buf + pb->len, data, (size_t)size);
    pb->len += (size_t)size;
}

ImageResult decode_to_png(const unsigned char* data, size_t len) {
    ImageResult result = {0};

    int w, h, channels;
    // Force 4 channels (RGBA) for consistent PNG output
    unsigned char* pixels = stbi_load_from_memory(data, (int)len, &w, &h, &channels, 4);
    if (!pixels) {
        return result;
    }

    PngBuffer pb = {0};
    pb.cap = 4096;
    pb.buf = (unsigned char*)malloc(pb.cap);
    if (!pb.buf) {
        stbi_image_free(pixels);
        return result;
    }

    int ok = stbi_write_png_to_func(png_write_cb, &pb, w, h, 4, pixels, w * 4);
    stbi_image_free(pixels);

    if (!ok || pb.len == 0) {
        free(pb.buf);
        return result;
    }

    result.png_data = pb.buf;
    result.png_len = pb.len;
    result.width = w;
    result.height = h;
    result.ok = 1;
    return result;
}

void free_image_result(ImageResult* r) {
    if (r && r->png_data) {
        free(r->png_data);
        r->png_data = NULL;
        r->png_len = 0;
    }
}

int image_dimensions(const unsigned char* data, size_t len, int* width, int* height) {
    int w, h, channels;
    int ok = stbi_info_from_memory(data, (int)len, &w, &h, &channels);
    if (!ok) return 0;
    *width = w;
    *height = h;
    return 1;
}
