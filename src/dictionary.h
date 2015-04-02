#include "web_buffer.h"

#ifndef NETDATA_DICTIONARY_H
#define NETDATA_DICTIONARY_H 1

typedef struct name_value {
	avl avl;				// the index - this has to be first!

	uint32_t hash;			// a simple hash to speed up searching
							// we first compare hashes, and only if the hashes are equal we do string comparisons

	char *name;
	char *value;

	struct name_value *next;
} NAME_VALUE;

typedef struct dictionary {
	NAME_VALUE *values;
	avl_tree values_index;
	pthread_rwlock_t rwlock;
} DICTIONARY;

extern DICTIONARY *dictionary_create(void);
extern void dictionary_destroy(DICTIONARY *dict);
extern void *dictionary_set(DICTIONARY *dict, const char *name, void *value, size_t value_len);
extern void *dictionary_get(DICTIONARY *dict, const char *name);

#endif /* NETDATA_DICTIONARY_H */
