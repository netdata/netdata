
#ifndef NETDATA_STRING_H
#define NETDATA_STRING_H 1

#include "../libnetdata.h"

// ----------------------------------------------------------------------------
// STRING implementation

typedef struct netdata_string STRING;
STRING *string_strdupz(const char *str);
STRING *string_dup(STRING *string);
void string_freez(STRING *string);
size_t string_strlen(STRING *string);
const char *string2str(STRING *string) NEVERNULL;
int string_strcmp(STRING *string, const char *s);

// keep common prefix/suffix and replace everything else with [x]
STRING *string_2way_merge(STRING *a, STRING *b);

static inline int string_cmp(STRING *s1, STRING *s2) {
    // STRINGs are deduplicated, so the same strings have the same pointer
    // when they differ, we do the typical strcmp() comparison
    return (s1 == s2)?0:strcmp(string2str(s1), string2str(s2));
}

void string_statistics(size_t *inserts, size_t *deletes, size_t *searches, size_t *entries, size_t *references, size_t *memory, size_t *duplications, size_t *releases);

int string_unittest(size_t entries);

#endif
