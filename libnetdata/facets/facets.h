#ifndef FACETS_H
#define FACETS_H 1

#include "../libnetdata.h"

typedef struct facets FACETS;

FACETS *facets_create(uint32_t items_to_return, usec_t anchor, const char *visible_keys, const char *facet_keys, const char *non_facet_keys);
void facets_destroy(FACETS *facets);

void facets_accepted_param(FACETS *facets, const char *param);

void facets_rows_begin(FACETS *facets);
void facets_row_finished(FACETS *facets, usec_t usec);

void facets_add_key_value(FACETS *facets, const char *key, const char *value);
void facets_add_key_value_length(FACETS *facets, const char *key, const char *value, size_t value_len);

void facets_report(FACETS *facets, BUFFER *wb);

#endif
