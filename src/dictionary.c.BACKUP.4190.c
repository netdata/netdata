#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <pthread.h>
#include <stdlib.h>
#include <string.h>

#include "avl.h"
#include "common.h"
#include "log.h"

#include "dictionary.h"

// ----------------------------------------------------------------------------
// name_value index

static int name_value_iterator(avl *a) { if(a) {}; return 0; }

static int name_value_compare(void* a, void* b) {
	if(((NAME_VALUE *)a)->hash < ((NAME_VALUE *)b)->hash) return -1;
	else if(((NAME_VALUE *)a)->hash > ((NAME_VALUE *)b)->hash) return 1;
	else return strcmp(((NAME_VALUE *)a)->name, ((NAME_VALUE *)b)->name);
}

#define dictionary_name_value_index_add_nolock(dict, nv) do { (dict)->inserts++; avl_insert(&((dict)->values_index), (avl *)(nv)); } while(0)
#define dictionary_name_value_index_del_nolock(dict, nv) do { (dict)->deletes++; avl_remove(&(dict->values_index), (avl *)(nv)); } while(0)

static inline NAME_VALUE *dictionary_name_value_index_find_nolock(DICTIONARY *dict, const char *name, uint32_t hash) {
	NAME_VALUE *result = NULL, tmp;
	tmp.hash = (hash)?hash:simple_hash(name);
	tmp.name = (char *)name;

	dict->searches++;
	avl_search(&(dict->values_index), (avl *)&tmp, name_value_iterator, (avl **)&result);
	return result;
}

static void dictionary_read_lock(DICTIONARY *dict) {
	if(likely(!(dict->flags & DICTIONARY_FLAG_SINGLE_THREADED)))
		pthread_rwlock_rdlock(&dict->rwlock);
}

static void dictionary_write_lock(DICTIONARY *dict) {
	if(likely(!(dict->flags & DICTIONARY_FLAG_SINGLE_THREADED)))
		pthread_rwlock_wrlock(&dict->rwlock);
}

static void dictionary_unlock(DICTIONARY *dict) {
	if(likely(!(dict->flags & DICTIONARY_FLAG_SINGLE_THREADED)))
		pthread_rwlock_unlock(&dict->rwlock);
}

// ----------------------------------------------------------------------------

static NAME_VALUE *dictionary_name_value_create_nolock(DICTIONARY *dict, const char *name, void *value, size_t value_len, uint32_t hash) {
	debug(D_DICTIONARY, "Creating name value entry for name '%s', value '%s'.", name, value);

	NAME_VALUE *nv = calloc(1, sizeof(NAME_VALUE));
	if(unlikely(!nv)) fatal("Cannot allocate name_value of size %z", sizeof(NAME_VALUE));

	nv->name = strdup(name);
	if(unlikely(!nv->name))
		fatal("Cannot allocate name_value.name of size %z", strlen(name));

	nv->hash = (hash)?hash:simple_hash(nv->name);

	if(dict->flags & DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE)
		nv->value = value;
	else {
		nv->value = malloc(value_len);
		if (unlikely(!nv->value))
			fatal("Cannot allocate name_value.value of size %z", value_len);

		memcpy(nv->value, value, value_len);
	}

	dictionary_write_lock(dict);

	// index it
	dictionary_name_value_index_add_nolock(dict, nv);

	dict->entries++;

	dictionary_unlock(dict);

	return nv;
}

static void dictionary_name_value_destroy_nolock(DICTIONARY *dict, NAME_VALUE *nv) {
	debug(D_DICTIONARY, "Destroying name value entry for name '%s'.", nv->name);

<<<<<<< HEAD
	pthread_rwlock_wrlock(&dict->rwlock);

  if(dict->values == nv) dict->values = nv->next;
	else {
		NAME_VALUE *n = dict->values;
		while(n && n->next && n->next != nv) nv = nv->next;
		if(!n || n->next != nv) {
			fatal("Cannot find name_value with name '%s' in dictionary.", nv->name);
			exit(1);
		}
		n->next = nv->next;
		nv->next = NULL;
	}
	pthread_rwlock_unlock(&dict->rwlock);

  // FIX: strdup() uses malloc to allocate new string. Must be freed.
  free(nv->name);

  free(nv->value);
=======
	dictionary_name_value_index_del_nolock(dict, nv);

	dict->entries--;

	free(nv->name);

	if(!(dict->flags & DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE))
		free(nv->value);

>>>>>>> master
	free(nv);
}

// ----------------------------------------------------------------------------

DICTIONARY *dictionary_create(uint32_t flags) {
	debug(D_DICTIONARY, "Creating dictionary.");

	DICTIONARY *dict = calloc(1, sizeof(DICTIONARY));
	if(unlikely(!dict)) fatal("Cannot allocate DICTIONARY");

	avl_init(&dict->values_index, name_value_compare);
	pthread_rwlock_init(&dict->rwlock, NULL);

	dict->flags = flags;

	return dict;
}

void dictionary_destroy(DICTIONARY *dict) {
	debug(D_DICTIONARY, "Destroying dictionary.");

	dictionary_write_lock(dict);

	while(dict->values_index.root)
		dictionary_name_value_destroy_nolock(dict, (NAME_VALUE *)dict->values_index.root);

	dictionary_unlock(dict);

	free(dict);
}

// ----------------------------------------------------------------------------

void *dictionary_set(DICTIONARY *dict, const char *name, void *value, size_t value_len) {
	debug(D_DICTIONARY, "SET dictionary entry with name '%s'.", name);

	uint32_t hash = simple_hash(name);

	dictionary_read_lock(dict);
	NAME_VALUE *nv = dictionary_name_value_index_find_nolock(dict, name, hash);
	dictionary_unlock(dict);

	if(unlikely(!nv)) {
		debug(D_DICTIONARY, "Dictionary entry with name '%s' not found. Creating a new one.", name);

		pthread_rwlock_wrlock(&dict->rwlock);
		nv = dictionary_name_value_create_nolock(dict, name, value, value_len, hash);

		if(unlikely(!nv))
			fatal("Cannot create name_value.");

		dictionary_unlock(dict);
	}
	else {
		debug(D_DICTIONARY, "Dictionary entry with name '%s' found. Changing its value.", name);

		if(dict->flags & DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE)
			nv->value = value;
		else {
			void *value = malloc(value_len),
					*old = nv->value;

			if(unlikely(!nv->value))
				fatal("Cannot allocate value of size %z", value_len);

			memcpy(value, value, value_len);
			nv->value = value;

			free(old);
		}
	}

	return nv->value;
}

void *dictionary_get(DICTIONARY *dict, const char *name) {
	debug(D_DICTIONARY, "GET dictionary entry with name '%s'.", name);

	dictionary_read_lock(dict);
	NAME_VALUE *nv = dictionary_name_value_index_find_nolock(dict, name, 0);
	dictionary_unlock(dict);

	if(unlikely(!nv)) {
		debug(D_DICTIONARY, "Not found dictionary entry with name '%s'.", name);
		return NULL;
	}

	debug(D_DICTIONARY, "Found dictionary entry with name '%s'.", name);
	return nv->value;
}

void dictionary_del(DICTIONARY *dict, const char *name) {
	debug(D_DICTIONARY, "DEL dictionary entry with name '%s'.", name);

	dictionary_read_lock(dict);
	NAME_VALUE *nv = dictionary_name_value_index_find_nolock(dict, name, 0);
	dictionary_unlock(dict);

	if(unlikely(!nv)) {
		debug(D_DICTIONARY, "Not found dictionary entry with name '%s'.", name);
		return;
	}

	debug(D_DICTIONARY, "Found dictionary entry with name '%s'.", name);
	pthread_rwlock_wrlock(&dict->rwlock);
	dictionary_name_value_destroy_nolock(dict, nv);
	dictionary_unlock(dict);
}
