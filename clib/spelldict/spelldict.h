#ifndef SPELLDICT_H
#define SPELLDICT_H

#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Opaque handle to a loaded Hunspell dictionary. */
typedef struct SpellDict SpellDict;

/* Load a Hunspell .dic file. Returns NULL on error. Caller must free with spelldict_free. */
SpellDict* spelldict_load(const char* path);

/* Release a dictionary returned by spelldict_load. */
void spelldict_free(SpellDict* dict);

/* Returns 1 if word (UTF-8, byte length len) is in the dictionary, 0 otherwise. */
int spelldict_contains(const SpellDict* dict, const char* word, size_t len);

/* Returns 1 if every letter codepoint in word appears in the dictionary's rune set, 0 otherwise. */
int spelldict_covers(const SpellDict* dict, const char* word, size_t len);

/*
 * Return up to limit spelling suggestions for word as a NULL-terminated array
 * of C strings. Caller must free the result with spelldict_free_suggestions.
 */
char** spelldict_suggest(const SpellDict* dict, const char* word, size_t len, int limit);

/* Free a suggestions array returned by spelldict_suggest. */
void spelldict_free_suggestions(char** suggestions);

#ifdef __cplusplus
}
#endif

#endif /* SPELLDICT_H */
