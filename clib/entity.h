#ifndef MD4C_ENTITY_H
#define MD4C_ENTITY_H

#include <stdlib.h>

typedef struct ENTITY_tag ENTITY;
struct ENTITY_tag {
    const char* name;
    unsigned codepoints[2];
};

const ENTITY* entity_lookup(const char* name, size_t name_size);

#endif
