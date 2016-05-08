#include <pthread.h>

#include "web_buffer.h"
#include "avl.h"

#ifndef NETDATA_DICTIONARY_H
#define NETDATA_DICTIONARY_H 1

typedef struct name_value {
	avl avl;				// the index - this has to be first!

	uint32_t hash;			// a simple hash to speed up searching
							// we first compare hashes, and only if the hashes are equal we do string comparisons
	char *name;
	void *value;
} NAME_VALUE;

typedef struct dictionary {
	uint32_t flags;

	unsigned long long inserts;
	unsigned long long deletes;
	unsigned long long searches;
	unsigned long long entries;

	avl_tree values_index;
	pthread_rwlock_t rwlock;
} DICTIONARY;

#define DICTIONARY_FLAG_DEFAULT					0x00000000
#define DICTIONARY_FLAG_SINGLE_THREADED			0x00000001
#define DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE	0x00000002

extern DICTIONARY *dictionary_create(uint32_t flags);
extern void dictionary_destroy(DICTIONARY *dict);
extern void *dictionary_set(DICTIONARY *dict, const char *name, void *value, size_t value_len);
extern void *dictionary_get(DICTIONARY *dict, const char *name);
extern void dictionary_del(DICTIONARY *dict, const char *name);

#endif /* NETDATA_DICTIONARY_H */
