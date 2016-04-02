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

#define name_value_index_add(dict, cv) avl_insert(&((dict)->values_index), (avl *)(cv))
#define name_value_index_del(dict, cv) avl_remove(&((dict)->values_index), (avl *)(cv))

static NAME_VALUE *dictionary_name_value_index_find(DICTIONARY *dict, const char *name, uint32_t hash) {
	NAME_VALUE *result = NULL, tmp;
	tmp.hash = (hash)?hash:simple_hash(name);
	tmp.name = (char *)name;

	avl_search(&(dict->values_index), (avl *)&tmp, name_value_iterator, (avl **)&result);
	return result;
}

// ----------------------------------------------------------------------------

static NAME_VALUE *dictionary_name_value_create(DICTIONARY *dict, const char *name, void *value, size_t value_len) {
	debug(D_DICTIONARY, "Creating name value entry for name '%s', value '%s'.", name, value);

	NAME_VALUE *nv = calloc(1, sizeof(NAME_VALUE));
	if(!nv) {
		fatal("Cannot allocate name_value of size %z", sizeof(NAME_VALUE));
		exit(1);
	}

	nv->name = strdup(name);
	if(!nv->name) fatal("Cannot allocate name_value.name of size %z", strlen(name));
	nv->hash = simple_hash(nv->name);

	nv->value = malloc(value_len);
	if(!nv->value) fatal("Cannot allocate name_value.value of size %z", value_len);
	memcpy(nv->value, value, value_len);

	// link it
	pthread_rwlock_wrlock(&dict->rwlock);
	nv->next = dict->values;
	dict->values = nv;
	pthread_rwlock_unlock(&dict->rwlock);

	// index it
	name_value_index_add(dict, nv);

	return nv;
}

static void dictionary_name_value_destroy(DICTIONARY *dict, NAME_VALUE *nv) {
	debug(D_DICTIONARY, "Destroying name value entry for name '%s'.", nv->name);

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

	free(nv->value);
	free(nv);
}

// ----------------------------------------------------------------------------

DICTIONARY *dictionary_create(void) {
	debug(D_DICTIONARY, "Creating dictionary.");

	DICTIONARY *dict = calloc(1, sizeof(DICTIONARY));
	if(!dict) {
		fatal("Cannot allocate DICTIONARY");
		exit(1);
	}

	avl_init(&dict->values_index, name_value_compare);
	pthread_rwlock_init(&dict->rwlock, NULL);

	return dict;
}

void dictionary_destroy(DICTIONARY *dict) {
	debug(D_DICTIONARY, "Destroying dictionary.");

	pthread_rwlock_wrlock(&dict->rwlock);
	while(dict->values) dictionary_name_value_destroy(dict, dict->values);
	pthread_rwlock_unlock(&dict->rwlock);

	free(dict);
}

// ----------------------------------------------------------------------------

void *dictionary_set(DICTIONARY *dict, const char *name, void *value, size_t value_len) {
	debug(D_DICTIONARY, "SET dictionary entry with name '%s'.", name);

	pthread_rwlock_rdlock(&dict->rwlock);
	NAME_VALUE *nv = dictionary_name_value_index_find(dict, name, 0);
	pthread_rwlock_unlock(&dict->rwlock);
	if(!nv) {
		debug(D_DICTIONARY, "Dictionary entry with name '%s' not found. Creating a new one.", name);
		nv = dictionary_name_value_create(dict, name, value, value_len);
		if(!nv) {
			fatal("Cannot create name_value.");
			exit(1);
		}
		return nv->value;
	}
	else {
		debug(D_DICTIONARY, "Dictionary entry with name '%s' found. Changing its value.", name);
		pthread_rwlock_wrlock(&dict->rwlock);
		void *old = nv->value;
		nv->value = malloc(value_len);
		if(!nv->value) fatal("Cannot allocate value of size %z", value_len);
		memcpy(nv->value, value, value_len);
		pthread_rwlock_unlock(&dict->rwlock);
		free(old);
	}

	return nv->value;
}

void *dictionary_get(DICTIONARY *dict, const char *name) {
	debug(D_DICTIONARY, "GET dictionary entry with name '%s'.", name);

	pthread_rwlock_rdlock(&dict->rwlock);
	NAME_VALUE *nv = dictionary_name_value_index_find(dict, name, 0);
	pthread_rwlock_unlock(&dict->rwlock);
	if(!nv) {
		debug(D_DICTIONARY, "Not found dictionary entry with name '%s'.", name);
		return NULL;
	}

	debug(D_DICTIONARY, "Found dictionary entry with name '%s'.", name);
	return nv->value;
}
