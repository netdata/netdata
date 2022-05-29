// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DICTIONARY_H
#define NETDATA_DICTIONARY_H 1

#include "../libnetdata.h"

typedef void DICTIONARY;

#define DICTIONARY_FLAG_NONE                    0x00
#define DICTIONARY_FLAG_SINGLE_THREADED         0x01
#define DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE   0x02
#define DICTIONARY_FLAG_NAME_LINK_DONT_CLONE    0x04
#define DICTIONARY_FLAG_WITH_STATISTICS         0x08
#define DICTIONARY_FLAG_DONT_OVERWRITE_VALUE    0x10

extern DICTIONARY *dictionary_create(uint8_t flags);
extern void dictionary_destroy(DICTIONARY *dict);
extern void *dictionary_set_with_name_ptr(DICTIONARY *dict, const char *name, void *value, size_t value_len, char **name_ptr) NEVERNULL;
#define dictionary_set(dict, name, value, value_len) dictionary_set_with_name_ptr(dict, name, value, value_len, NULL)
extern void *dictionary_get(DICTIONARY *dict, const char *name);
extern int dictionary_del(DICTIONARY *dict, const char *name);

extern int dictionary_walkthrough(DICTIONARY *dict, int (*callback)(void *value, void *data), void *data);
extern int dictionary_walkthrough_with_name(DICTIONARY *dict, int (*callback)(const char *name, void *value, void *data), void *data);

extern size_t dictionary_allocated_memory(DICTIONARY *dict);
extern size_t dictionary_entries(DICTIONARY *dict);

extern int dictionary_unittest(size_t entries);

#endif /* NETDATA_DICTIONARY_H */
