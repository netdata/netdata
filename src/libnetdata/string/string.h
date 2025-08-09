
#ifndef NETDATA_STRING_H
#define NETDATA_STRING_H 1

#include "../libnetdata.h"

// ----------------------------------------------------------------------------
// STRING implementation

typedef struct netdata_string STRING;

STRING *string_strdupz(const char *str);
STRING *string_strndupz(const char *str, size_t len);

STRING *string_dup(STRING *string);
void string_freez(STRING *string);
size_t string_strlen(const STRING *string);
const char *string2str(const STRING *string) NEVERNULL;
bool string_ends_with_string(const STRING *whole, const STRING *end);
bool string_starts_with_string(const STRING *whole, const STRING *end);
bool string_ends_with_string_nocase(const STRING *whole, const STRING *end);
bool string_starts_with_string_nocase(const STRING *whole, const STRING *prefix);
bool string_equals_string_nocase(const STRING *a, const STRING *b);
size_t string_destroy(void);

// keep common prefix/suffix and replace everything else with [x]
STRING *string_2way_merge(STRING *a, STRING *b);

static inline int string_cmp(STRING *s1, STRING *s2) {
    // STRINGs are deduplicated, so the same strings have the same pointer
    // when they differ, we do the typical strcmp() comparison
    return (s1 == s2)?0:strcmp(string2str(s1), string2str(s2));
}

static inline int string_strcmp(STRING *string, const char *s) {
    return strcmp(string2str(string), s);
}

static inline int string_strncmp(STRING *string, const char *s, size_t n) {
    return strncmp(string2str(string), s, n);
}

void string_statistics(size_t *inserts, size_t *deletes, size_t *searches, size_t *entries, size_t *references, size_t *memory, size_t *memory_index, size_t *duplications, size_t *releases);

int string_unittest(size_t entries);

static inline void cleanup_string_pp(STRING **stringpp) {
    if(stringpp)
        string_freez(*stringpp);
}

void string_init(void);

#define CLEAN_STRING _cleanup_(cleanup_string_pp) STRING

#endif
