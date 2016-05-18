#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <stddef.h>
#include <string.h>
#include <unistd.h>
#include <time.h>
#include <sys/time.h>
#include <sys/mman.h>
#include <pthread.h>
#include <errno.h>
#include <ctype.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <stdlib.h>

#include "common.h"
#include "log.h"
#include "appconfig.h"

#include "rrd.h"

#define RRD_DEFAULT_GAP_INTERPOLATIONS 1

// ----------------------------------------------------------------------------
// globals

// if not zero it gives the time (in seconds) to remove un-updated dimensions
// DO NOT ENABLE
// if dimensions are removed, the chart generation will have to run again
int rrd_delete_unupdated_dimensions = 0;

int rrd_update_every = UPDATE_EVERY;
int rrd_default_history_entries = RRD_DEFAULT_HISTORY_ENTRIES;

RRDSET *rrdset_root = NULL;
pthread_rwlock_t rrdset_root_rwlock = PTHREAD_RWLOCK_INITIALIZER;

int rrd_memory_mode = RRD_MEMORY_MODE_SAVE;


// ----------------------------------------------------------------------------
// RRDSET index

static int rrdset_compare(void* a, void* b) {
	if(((RRDSET *)a)->hash < ((RRDSET *)b)->hash) return -1;
	else if(((RRDSET *)a)->hash > ((RRDSET *)b)->hash) return 1;
	else return strcmp(((RRDSET *)a)->id, ((RRDSET *)b)->id);
}

avl_tree_lock rrdset_root_index = {
		{ NULL, rrdset_compare },
		AVL_LOCK_INITIALIZER
};

#define rrdset_index_add(st) avl_insert_lock(&rrdset_root_index, (avl *)(st))
#define rrdset_index_del(st) avl_remove_lock(&rrdset_root_index, (avl *)(st))

static RRDSET *rrdset_index_find(const char *id, uint32_t hash) {
	RRDSET tmp;
	strncpyz(tmp.id, id, RRD_ID_LENGTH_MAX);
	tmp.hash = (hash)?hash:simple_hash(tmp.id);

	return (RRDSET *)avl_search_lock(&(rrdset_root_index), (avl *) &tmp);
}

// ----------------------------------------------------------------------------
// RRDSET name index

#define rrdset_from_avlname(avlname_ptr) ((RRDSET *)((avlname_ptr) - offsetof(RRDSET, avlname)))

static int rrdset_compare_name(void* a, void* b) {
	RRDSET *A = rrdset_from_avlname(a);
	RRDSET *B = rrdset_from_avlname(b);

	// fprintf(stderr, "COMPARING: %s with %s\n", A->name, B->name);

	if(A->hash_name < B->hash_name) return -1;
	else if(A->hash_name > B->hash_name) return 1;
	else return strcmp(A->name, B->name);
}

avl_tree_lock rrdset_root_index_name = {
		{ NULL, rrdset_compare_name },
		AVL_LOCK_INITIALIZER
};

int rrdset_index_add_name(RRDSET *st) {
	// fprintf(stderr, "ADDING: %s (name: %s)\n", st->id, st->name);
	return avl_insert_lock(&rrdset_root_index_name, (avl *) (&st->avlname));
}

#define rrdset_index_del_name(st) avl_remove_lock(&rrdset_root_index_name, (avl *)(&st->avlname))

static RRDSET *rrdset_index_find_name(const char *name, uint32_t hash) {
	void *result = NULL;
	RRDSET tmp;
	tmp.name = name;
	tmp.hash_name = (hash)?hash:simple_hash(tmp.name);

	// fprintf(stderr, "SEARCHING: %s\n", name);
	result = avl_search_lock(&(rrdset_root_index_name), (avl *) (&(tmp.avlname)));
	if(result) {
		RRDSET *st = rrdset_from_avlname(result);
		if(strcmp(st->magic, RRDSET_MAGIC))
			error("Search for RRDSET %s returned an invalid RRDSET %s (name %s)", name, st->id, st->name);

		// fprintf(stderr, "FOUND: %s\n", name);
		return rrdset_from_avlname(result);
	}
	// fprintf(stderr, "NOT FOUND: %s\n", name);
	return NULL;
}


// ----------------------------------------------------------------------------
// RRDDIM index

static int rrddim_compare(void* a, void* b) {
	if(((RRDDIM *)a)->hash < ((RRDDIM *)b)->hash) return -1;
	else if(((RRDDIM *)a)->hash > ((RRDDIM *)b)->hash) return 1;
	else return strcmp(((RRDDIM *)a)->id, ((RRDDIM *)b)->id);
}

#define rrddim_index_add(st, rd) avl_insert_lock(&((st)->dimensions_index), (avl *)(rd))
#define rrddim_index_del(st,rd ) avl_remove_lock(&((st)->dimensions_index), (avl *)(rd))

static RRDDIM *rrddim_index_find(RRDSET *st, const char *id, uint32_t hash) {
	RRDDIM tmp;
	strncpyz(tmp.id, id, RRD_ID_LENGTH_MAX);
	tmp.hash = (hash)?hash:simple_hash(tmp.id);

	return (RRDDIM *)avl_search_lock(&(st->dimensions_index), (avl *) &tmp);
}

// ----------------------------------------------------------------------------
// chart types

int rrdset_type_id(const char *name)
{
	if(unlikely(strcmp(name, RRDSET_TYPE_AREA_NAME) == 0)) return RRDSET_TYPE_AREA;
	else if(unlikely(strcmp(name, RRDSET_TYPE_STACKED_NAME) == 0)) return RRDSET_TYPE_STACKED;
	else if(unlikely(strcmp(name, RRDSET_TYPE_LINE_NAME) == 0)) return RRDSET_TYPE_LINE;
	return RRDSET_TYPE_LINE;
}

const char *rrdset_type_name(int chart_type)
{
	static char line[] = RRDSET_TYPE_LINE_NAME;
	static char area[] = RRDSET_TYPE_AREA_NAME;
	static char stacked[] = RRDSET_TYPE_STACKED_NAME;

	switch(chart_type) {
		case RRDSET_TYPE_LINE:
			return line;

		case RRDSET_TYPE_AREA:
			return area;

		case RRDSET_TYPE_STACKED:
			return stacked;
	}
	return line;
}

// ----------------------------------------------------------------------------
// load / save

const char *rrd_memory_mode_name(int id)
{
	static const char ram[] = RRD_MEMORY_MODE_RAM_NAME;
	static const char map[] = RRD_MEMORY_MODE_MAP_NAME;
	static const char save[] = RRD_MEMORY_MODE_SAVE_NAME;

	switch(id) {
		case RRD_MEMORY_MODE_RAM:
			return ram;

		case RRD_MEMORY_MODE_MAP:
			return map;

		case RRD_MEMORY_MODE_SAVE:
		default:
			return save;
	}

	return save;
}

int rrd_memory_mode_id(const char *name)
{
	if(unlikely(!strcmp(name, RRD_MEMORY_MODE_RAM_NAME)))
		return RRD_MEMORY_MODE_RAM;
	else if(unlikely(!strcmp(name, RRD_MEMORY_MODE_MAP_NAME)))
		return RRD_MEMORY_MODE_MAP;

	return RRD_MEMORY_MODE_SAVE;
}

// ----------------------------------------------------------------------------
// algorithms types

int rrddim_algorithm_id(const char *name)
{
	if(strcmp(name, RRDDIM_INCREMENTAL_NAME) == 0) 			return RRDDIM_INCREMENTAL;
	if(strcmp(name, RRDDIM_ABSOLUTE_NAME) == 0) 			return RRDDIM_ABSOLUTE;
	if(strcmp(name, RRDDIM_PCENT_OVER_ROW_TOTAL_NAME) == 0) 		return RRDDIM_PCENT_OVER_ROW_TOTAL;
	if(strcmp(name, RRDDIM_PCENT_OVER_DIFF_TOTAL_NAME) == 0) 	return RRDDIM_PCENT_OVER_DIFF_TOTAL;
	return RRDDIM_ABSOLUTE;
}

const char *rrddim_algorithm_name(int chart_type)
{
	static char absolute[] = RRDDIM_ABSOLUTE_NAME;
	static char incremental[] = RRDDIM_INCREMENTAL_NAME;
	static char percentage_of_absolute_row[] = RRDDIM_PCENT_OVER_ROW_TOTAL_NAME;
	static char percentage_of_incremental_row[] = RRDDIM_PCENT_OVER_DIFF_TOTAL_NAME;

	switch(chart_type) {
		case RRDDIM_ABSOLUTE:
			return absolute;

		case RRDDIM_INCREMENTAL:
			return incremental;

		case RRDDIM_PCENT_OVER_ROW_TOTAL:
			return percentage_of_absolute_row;

		case RRDDIM_PCENT_OVER_DIFF_TOTAL:
			return percentage_of_incremental_row;
	}
	return absolute;
}

// ----------------------------------------------------------------------------
// chart names

char *rrdset_strncpyz_name(char *to, const char *from, size_t length)
{
	char c, *p = to;

	while (length-- && (c = *from++)) {
		if(c != '.' && !isalnum(c))
			c = '_';

		*p++ = c;
	}

	*p = '\0';

	return to;
}

void rrdset_set_name(RRDSET *st, const char *name)
{
	debug(D_RRD_CALLS, "rrdset_set_name() old: %s, new: %s", st->name, name);

	if(st->name) rrdset_index_del_name(st);

	char b[CONFIG_MAX_VALUE + 1];
	char n[RRD_ID_LENGTH_MAX + 1];

	snprintfz(n, RRD_ID_LENGTH_MAX, "%s.%s", st->type, name);
	rrdset_strncpyz_name(b, n, CONFIG_MAX_VALUE);
	st->name = config_get(st->id, "name", b);
	st->hash_name = simple_hash(st->name);

	rrdset_index_add_name(st);
}

// ----------------------------------------------------------------------------
// cache directory

char *rrdset_cache_dir(const char *id)
{
	char *ret = NULL;

	static char *cache_dir = NULL;
	if(!cache_dir) cache_dir = config_get("global", "cache directory", CACHE_DIR);

	char b[FILENAME_MAX + 1];
	char n[FILENAME_MAX + 1];
	rrdset_strncpyz_name(b, id, FILENAME_MAX);

	snprintfz(n, FILENAME_MAX, "%s/%s", cache_dir, b);
	ret = config_get(id, "cache directory", n);

	if(rrd_memory_mode == RRD_MEMORY_MODE_MAP || rrd_memory_mode == RRD_MEMORY_MODE_SAVE) {
		int r = mkdir(ret, 0775);
		if(r != 0 && errno != EEXIST)
			error("Cannot create directory '%s'", ret);
	}

	return ret;
}

// ----------------------------------------------------------------------------
// core functions

void rrdset_reset(RRDSET *st)
{
	debug(D_RRD_CALLS, "rrdset_reset() %s", st->name);

	st->last_collected_time.tv_sec = 0;
	st->last_collected_time.tv_usec = 0;
	st->last_updated.tv_sec = 0;
	st->last_updated.tv_usec = 0;
	st->current_entry = 0;
	st->counter = 0;
	st->counter_done = 0;

	RRDDIM *rd;
	for(rd = st->dimensions; rd ; rd = rd->next) {
		rd->last_collected_time.tv_sec = 0;
		rd->last_collected_time.tv_usec = 0;
		rd->counter = 0;
		bzero(rd->values, rd->entries * sizeof(storage_number));
	}
}

RRDSET *rrdset_create(const char *type, const char *id, const char *name, const char *family, const char *context, const char *title, const char *units, long priority, int update_every, int chart_type)
{
	if(!type || !type[0]) {
		fatal("Cannot create rrd stats without a type.");
		return NULL;
	}

	if(!id || !id[0]) {
		fatal("Cannot create rrd stats without an id.");
		return NULL;
	}

	char fullid[RRD_ID_LENGTH_MAX + 1];
	char fullfilename[FILENAME_MAX + 1];
	RRDSET *st = NULL;

	snprintfz(fullid, RRD_ID_LENGTH_MAX, "%s.%s", type, id);

	st = rrdset_find(fullid);
	if(st) {
		error("Cannot create rrd stats for '%s', it already exists.", fullid);
		return st;
	}

	long entries = config_get_number(fullid, "history", rrd_default_history_entries);
	if(entries < 5) entries = config_set_number(fullid, "history", 5);
	if(entries > RRD_HISTORY_ENTRIES_MAX) entries = config_set_number(fullid, "history", RRD_HISTORY_ENTRIES_MAX);

	int enabled = config_get_boolean(fullid, "enabled", 1);
	if(!enabled) entries = 5;

	unsigned long size = sizeof(RRDSET);
	char *cache_dir = rrdset_cache_dir(fullid);

	debug(D_RRD_CALLS, "Creating RRD_STATS for '%s.%s'.", type, id);

	snprintfz(fullfilename, FILENAME_MAX, "%s/main.db", cache_dir);
	if(rrd_memory_mode != RRD_MEMORY_MODE_RAM) st = (RRDSET *)mymmap(fullfilename, size, ((rrd_memory_mode == RRD_MEMORY_MODE_MAP)?MAP_SHARED:MAP_PRIVATE), 0);
	if(st) {
		if(strcmp(st->magic, RRDSET_MAGIC) != 0) {
			errno = 0;
			info("Initializing file %s.", fullfilename);
			bzero(st, size);
		}
		else if(strcmp(st->id, fullid) != 0) {
			errno = 0;
			error("File %s contents are not for chart %s. Clearing it.", fullfilename, fullid);
			// munmap(st, size);
			// st = NULL;
			bzero(st, size);
		}
		else if(st->memsize != size || st->entries != entries) {
			errno = 0;
			error("File %s does not have the desired size. Clearing it.", fullfilename);
			bzero(st, size);
		}
		else if(st->update_every != update_every) {
			errno = 0;
			error("File %s does not have the desired update frequency. Clearing it.", fullfilename);
			bzero(st, size);
		}
		else if((time(NULL) - st->last_updated.tv_sec) > update_every * entries) {
			errno = 0;
			error("File %s is too old. Clearing it.", fullfilename);
			bzero(st, size);
		}
	}

	if(st) {
		st->name = NULL;
		st->type = NULL;
		st->family = NULL;
		st->context = NULL;
		st->title = NULL;
		st->units = NULL;
		st->dimensions = NULL;
		st->next = NULL;
		st->mapped = rrd_memory_mode;
	}
	else {
		st = calloc(1, size);
		if(!st) {
			fatal("Cannot allocate memory for RRD_STATS %s.%s", type, id);
			return NULL;
		}
		st->mapped = RRD_MEMORY_MODE_RAM;
	}
	st->memsize = size;
	st->entries = entries;
	st->update_every = update_every;

	strcpy(st->cache_filename, fullfilename);
	strcpy(st->magic, RRDSET_MAGIC);

	strcpy(st->id, fullid);
	st->hash = simple_hash(st->id);

	st->cache_dir = cache_dir;

	st->chart_type = rrdset_type_id(config_get(st->id, "chart type", rrdset_type_name(chart_type)));
	st->type       = config_get(st->id, "type", type);
	st->family     = config_get(st->id, "family", family?family:st->type);
	st->context    = config_get(st->id, "context", context?context:st->id);
	st->units      = config_get(st->id, "units", units?units:"");

	st->priority = config_get_number(st->id, "priority", priority);
	st->enabled = enabled;

	st->isdetail = 0;
	st->debug = 0;

	st->last_collected_time.tv_sec = 0;
	st->last_collected_time.tv_usec = 0;
	st->counter_done = 0;

	st->gap_when_lost_iterations_above = (int) (
			config_get_number(st->id, "gap when lost iterations above", RRD_DEFAULT_GAP_INTERPOLATIONS) + 2);

	avl_init_lock(&st->dimensions_index, rrddim_compare);

	pthread_rwlock_init(&st->rwlock, NULL);
	pthread_rwlock_wrlock(&rrdset_root_rwlock);

	if(name && *name) rrdset_set_name(st, name);
	else rrdset_set_name(st, id);

	{
		char varvalue[CONFIG_MAX_VALUE + 1];
		snprintfz(varvalue, CONFIG_MAX_VALUE, "%s (%s)", title?title:"", st->name);
		st->title = config_get(st->id, "title", varvalue);
	}

	st->next = rrdset_root;
	rrdset_root = st;

	rrdset_index_add(st);

	pthread_rwlock_unlock(&rrdset_root_rwlock);

	return(st);
}

RRDDIM *rrddim_add(RRDSET *st, const char *id, const char *name, long multiplier, long divisor, int algorithm)
{
	char filename[FILENAME_MAX + 1];
	char fullfilename[FILENAME_MAX + 1];

	char varname[CONFIG_MAX_NAME + 1];
	RRDDIM *rd = NULL;
	unsigned long size = sizeof(RRDDIM) + (st->entries * sizeof(storage_number));

	debug(D_RRD_CALLS, "Adding dimension '%s/%s'.", st->id, id);

	rrdset_strncpyz_name(filename, id, FILENAME_MAX);
	snprintfz(fullfilename, FILENAME_MAX, "%s/%s.db", st->cache_dir, filename);
	if(rrd_memory_mode != RRD_MEMORY_MODE_RAM) rd = (RRDDIM *)mymmap(fullfilename, size, ((rrd_memory_mode == RRD_MEMORY_MODE_MAP)?MAP_SHARED:MAP_PRIVATE), 1);
	if(rd) {
		struct timeval now;
		gettimeofday(&now, NULL);

		if(strcmp(rd->magic, RRDDIMENSION_MAGIC) != 0) {
			errno = 0;
			info("Initializing file %s.", fullfilename);
			bzero(rd, size);
		}
		else if(rd->memsize != size) {
			errno = 0;
			error("File %s does not have the desired size. Clearing it.", fullfilename);
			bzero(rd, size);
		}
		else if(rd->multiplier != multiplier) {
			errno = 0;
			error("File %s does not have the same multiplier. Clearing it.", fullfilename);
			bzero(rd, size);
		}
		else if(rd->divisor != divisor) {
			errno = 0;
			error("File %s does not have the same divisor. Clearing it.", fullfilename);
			bzero(rd, size);
		}
		else if(rd->algorithm != algorithm) {
			errno = 0;
			error("File %s does not have the same algorithm. Clearing it.", fullfilename);
			bzero(rd, size);
		}
		else if(rd->update_every != st->update_every) {
			errno = 0;
			error("File %s does not have the same refresh frequency. Clearing it.", fullfilename);
			bzero(rd, size);
		}
		else if(usecdiff(&now, &rd->last_collected_time) > (rd->entries * rd->update_every * 1000000ULL)) {
			errno = 0;
			error("File %s is too old. Clearing it.", fullfilename);
			bzero(rd, size);
		}
		else if(strcmp(rd->id, id) != 0) {
			errno = 0;
			error("File %s contents are not for dimension %s. Clearing it.", fullfilename, id);
			// munmap(rd, size);
			// rd = NULL;
			bzero(rd, size);
		}
	}

	if(rd) {
		// we have a file mapped for rd
		rd->mapped = rrd_memory_mode;
		rd->flags = 0x00000000;
		rd->next = NULL;
		rd->name = NULL;
	}
	else {
		// if we didn't manage to get a mmap'd dimension, just create one

		rd = calloc(1, size);
		if(!rd) {
			fatal("Cannot allocate RRD_DIMENSION %s/%s.", st->id, id);
			return NULL;
		}

		rd->mapped = RRD_MEMORY_MODE_RAM;
	}
	rd->memsize = size;

	strcpy(rd->magic, RRDDIMENSION_MAGIC);
	strcpy(rd->cache_filename, fullfilename);
	strncpyz(rd->id, id, RRD_ID_LENGTH_MAX);
	rd->hash = simple_hash(rd->id);

	snprintfz(varname, CONFIG_MAX_NAME, "dim %s name", rd->id);
	rd->name = config_get(st->id, varname, (name && *name)?name:rd->id);

	snprintfz(varname, CONFIG_MAX_NAME, "dim %s algorithm", rd->id);
	rd->algorithm = rrddim_algorithm_id(config_get(st->id, varname, rrddim_algorithm_name(algorithm)));

	snprintfz(varname, CONFIG_MAX_NAME, "dim %s multiplier", rd->id);
	rd->multiplier = config_get_number(st->id, varname, multiplier);

	snprintfz(varname, CONFIG_MAX_NAME, "dim %s divisor", rd->id);
	rd->divisor = config_get_number(st->id, varname, divisor);
	if(!rd->divisor) rd->divisor = 1;

	rd->entries = st->entries;
	rd->update_every = st->update_every;

	// prevent incremental calculation spikes
	rd->counter = 0;

	// append this dimension
	pthread_rwlock_wrlock(&st->rwlock);
	if(!st->dimensions)
		st->dimensions = rd;
	else {
		RRDDIM *td = st->dimensions;
		for(; td->next; td = td->next) ;
		td->next = rd;
	}
	pthread_rwlock_unlock(&st->rwlock);

	rrddim_index_add(st, rd);

	return(rd);
}

void rrddim_set_name(RRDSET *st, RRDDIM *rd, const char *name)
{
	debug(D_RRD_CALLS, "rrddim_set_name() %s.%s", st->name, rd->name);

	char varname[CONFIG_MAX_NAME + 1];
	snprintfz(varname, CONFIG_MAX_NAME, "dim %s name", rd->id);
	config_set_default(st->id, varname, name);
}

void rrddim_free(RRDSET *st, RRDDIM *rd)
{
	debug(D_RRD_CALLS, "rrddim_free() %s.%s", st->name, rd->name);

	RRDDIM *i, *last = NULL;
	for(i = st->dimensions; i && i != rd ; i = i->next) last = i;

	if(!i) {
		error("Request to free dimension %s.%s but it is not linked.", st->id, rd->name);
		return;
	}

	if(last) last->next = rd->next;
	else st->dimensions = rd->next;
	rd->next = NULL;

	rrddim_index_del(st, rd);

	// free(rd->annotations);
	if(rd->mapped == RRD_MEMORY_MODE_SAVE) {
		debug(D_RRD_CALLS, "Saving dimension '%s' to '%s'.", rd->name, rd->cache_filename);
		savememory(rd->cache_filename, rd, rd->memsize);

		debug(D_RRD_CALLS, "Unmapping dimension '%s'.", rd->name);
		munmap(rd, rd->memsize);
	}
	else if(rd->mapped == RRD_MEMORY_MODE_MAP) {
		debug(D_RRD_CALLS, "Unmapping dimension '%s'.", rd->name);
		munmap(rd, rd->memsize);
	}
	else {
		debug(D_RRD_CALLS, "Removing dimension '%s'.", rd->name);
		free(rd);
	}
}

void rrdset_free_all(void)
{
	info("Freeing all memory...");

	RRDSET *st;
	for(st = rrdset_root; st ;) {
		RRDSET *next = st->next;

		while(st->dimensions)
			rrddim_free(st, st->dimensions);

		rrdset_index_del(st);

		if(st->mapped == RRD_MEMORY_MODE_SAVE) {
			debug(D_RRD_CALLS, "Saving stats '%s' to '%s'.", st->name, st->cache_filename);
			savememory(st->cache_filename, st, st->memsize);

			debug(D_RRD_CALLS, "Unmapping stats '%s'.", st->name);
			munmap(st, st->memsize);
		}
		else if(st->mapped == RRD_MEMORY_MODE_MAP) {
			debug(D_RRD_CALLS, "Unmapping stats '%s'.", st->name);
			munmap(st, st->memsize);
		}
		else
			free(st);

		st = next;
	}
	rrdset_root = NULL;

	info("Memory cleanup completed...");
}

void rrdset_save_all(void)
{
	debug(D_RRD_CALLS, "rrdset_save_all()");

	// let it log a few error messages
	error_log_limit_reset();

	RRDSET *st;
	RRDDIM *rd;

	pthread_rwlock_wrlock(&rrdset_root_rwlock);
	for(st = rrdset_root; st ; st = st->next) {
		pthread_rwlock_wrlock(&st->rwlock);

		if(st->mapped == RRD_MEMORY_MODE_SAVE) {
			debug(D_RRD_CALLS, "Saving stats '%s' to '%s'.", st->name, st->cache_filename);
			savememory(st->cache_filename, st, st->memsize);
		}

		for(rd = st->dimensions; rd ; rd = rd->next) {
			if(likely(rd->mapped == RRD_MEMORY_MODE_SAVE)) {
				debug(D_RRD_CALLS, "Saving dimension '%s' to '%s'.", rd->name, rd->cache_filename);
				savememory(rd->cache_filename, rd, rd->memsize);
			}
		}

		pthread_rwlock_unlock(&st->rwlock);
	}
	pthread_rwlock_unlock(&rrdset_root_rwlock);
}


RRDSET *rrdset_find(const char *id)
{
	debug(D_RRD_CALLS, "rrdset_find() for chart %s", id);

	RRDSET *st = rrdset_index_find(id, 0);
	return(st);
}

RRDSET *rrdset_find_bytype(const char *type, const char *id)
{
	debug(D_RRD_CALLS, "rrdset_find_bytype() for chart %s.%s", type, id);

	char buf[RRD_ID_LENGTH_MAX + 1];

	strncpyz(buf, type, RRD_ID_LENGTH_MAX - 1);
	strcat(buf, ".");
	int len = (int) strlen(buf);
	strncpyz(&buf[len], id, (size_t) (RRD_ID_LENGTH_MAX - len));

	return(rrdset_find(buf));
}

RRDSET *rrdset_find_byname(const char *name)
{
	debug(D_RRD_CALLS, "rrdset_find_byname() for chart %s", name);

	RRDSET *st = rrdset_index_find_name(name, 0);
	return(st);
}

RRDDIM *rrddim_find(RRDSET *st, const char *id)
{
	debug(D_RRD_CALLS, "rrddim_find() for chart %s, dimension %s", st->name, id);

	return rrddim_index_find(st, id, 0);
}

int rrddim_hide(RRDSET *st, const char *id)
{
	debug(D_RRD_CALLS, "rrddim_hide() for chart %s, dimension %s", st->name, id);

	RRDDIM *rd = rrddim_find(st, id);
	if(unlikely(!rd)) {
		error("Cannot find dimension with id '%s' on stats '%s' (%s).", id, st->name, st->id);
		return 1;
	}

	rd->flags |= RRDDIM_FLAG_HIDDEN;
	return 0;
}

int rrddim_unhide(RRDSET *st, const char *id)
{
	debug(D_RRD_CALLS, "rrddim_unhide() for chart %s, dimension %s", st->name, id);

	RRDDIM *rd = rrddim_find(st, id);
	if(unlikely(!rd)) {
		error("Cannot find dimension with id '%s' on stats '%s' (%s).", id, st->name, st->id);
		return 1;
	}

	if(rd->flags & RRDDIM_FLAG_HIDDEN) rd->flags ^= RRDDIM_FLAG_HIDDEN;
	return 0;
}

collected_number rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, collected_number value)
{
	debug(D_RRD_CALLS, "rrddim_set_by_pointer() for chart %s, dimension %s, value " COLLECTED_NUMBER_FORMAT, st->name, rd->name, value);

	gettimeofday(&rd->last_collected_time, NULL);
	rd->collected_value = value;
	rd->updated = 1;
	rd->counter++;

	return rd->last_collected_value;
}

collected_number rrddim_set(RRDSET *st, const char *id, collected_number value)
{
	RRDDIM *rd = rrddim_find(st, id);
	if(unlikely(!rd)) {
		error("Cannot find dimension with id '%s' on stats '%s' (%s).", id, st->name, st->id);
		return 0;
	}

	return rrddim_set_by_pointer(st, rd, value);
}

void rrdset_next_usec(RRDSET *st, unsigned long long microseconds)
{
	if(!microseconds) rrdset_next(st);
	else {
		debug(D_RRD_CALLS, "rrdset_next_usec() for chart %s with microseconds %llu", st->name, microseconds);

		if(unlikely(st->debug)) debug(D_RRD_STATS, "%s: NEXT: %llu microseconds", st->name, microseconds);
		st->usec_since_last_update = microseconds;
	}
}

void rrdset_next(RRDSET *st)
{
	unsigned long long microseconds = 0;

	if(likely(st->last_collected_time.tv_sec)) {
		struct timeval now;
		gettimeofday(&now, NULL);
		microseconds = usecdiff(&now, &st->last_collected_time);
	}
	// prevent infinite loop
	else microseconds = st->update_every * 1000000ULL;

	rrdset_next_usec(st, microseconds);
}

void rrdset_next_plugins(RRDSET *st)
{
	rrdset_next(st);
}

unsigned long long rrdset_done(RRDSET *st)
{
	debug(D_RRD_CALLS, "rrdset_done() for chart %s", st->name);

	RRDDIM *rd, *last;
	int oldstate, store_this_entry = 1, first_entry = 0;
	unsigned long long last_ut, now_ut, next_ut, stored_entries = 0;

	if(unlikely(pthread_setcancelstate(PTHREAD_CANCEL_DISABLE, &oldstate) != 0))
		error("Cannot set pthread cancel state to DISABLE.");

	// a read lock is OK here
	pthread_rwlock_rdlock(&st->rwlock);

	// enable the chart, if it was disabled
	if(unlikely(rrd_delete_unupdated_dimensions) && !st->enabled)
		st->enabled = 1;

	// check if the chart has a long time to be updated
	if(unlikely(st->usec_since_last_update > st->entries * st->update_every * 1000000ULL)) {
		info("%s: took too long to be updated (%0.3Lf secs). Reseting it.", st->name, (long double)(st->usec_since_last_update / 1000000.0));
		rrdset_reset(st);
		st->usec_since_last_update = st->update_every * 1000000ULL;
		first_entry = 1;
	}
	if(unlikely(st->debug)) debug(D_RRD_STATS, "%s: microseconds since last update: %llu", st->name, st->usec_since_last_update);

	// set last_collected_time
	if(unlikely(!st->last_collected_time.tv_sec)) {
		// it is the first entry
		// set the last_collected_time to now
		gettimeofday(&st->last_collected_time, NULL);

		// the first entry should not be stored
		store_this_entry = 0;
		first_entry = 1;

		if(unlikely(st->debug)) debug(D_RRD_STATS, "%s: has not set last_collected_time. Setting it now. Will not store the next entry.", st->name);
	}
	else {
		// it is not the first entry
		// calculate the proper last_collected_time, using usec_since_last_update
		unsigned long long ut = st->last_collected_time.tv_sec * 1000000ULL + st->last_collected_time.tv_usec + st->usec_since_last_update;
		st->last_collected_time.tv_sec = (time_t) (ut / 1000000ULL);
		st->last_collected_time.tv_usec = (useconds_t) (ut % 1000000ULL);
	}

	// if this set has not been updated in the past
	// we fake the last_update time to be = now - usec_since_last_update
	if(unlikely(!st->last_updated.tv_sec)) {
		// it has never been updated before
		// set a fake last_updated, in the past using usec_since_last_update
		unsigned long long ut = st->last_collected_time.tv_sec * 1000000ULL + st->last_collected_time.tv_usec - st->usec_since_last_update;
		st->last_updated.tv_sec = (time_t) (ut / 1000000ULL);
		st->last_updated.tv_usec = (useconds_t) (ut % 1000000ULL);

		// the first entry should not be stored
		store_this_entry = 0;
		first_entry = 1;

		if(unlikely(st->debug)) debug(D_RRD_STATS, "%s: initializing last_updated to now - %llu microseconds (%0.3Lf). Will not store the next entry.", st->name, st->usec_since_last_update, (long double)ut/1000000.0);
	}

	// check if we will re-write the entire data set
	if(unlikely(usecdiff(&st->last_collected_time, &st->last_updated) > st->update_every * st->entries * 1000000ULL)) {
		info("%s: too old data (last updated at %u.%u, last collected at %u.%u). Reseting it. Will not store the next entry.", st->name, st->last_updated.tv_sec, st->last_updated.tv_usec, st->last_collected_time.tv_sec, st->last_collected_time.tv_usec);
		rrdset_reset(st);

		st->usec_since_last_update = st->update_every * 1000000ULL;

		gettimeofday(&st->last_collected_time, NULL);

		unsigned long long ut = st->last_collected_time.tv_sec * 1000000ULL + st->last_collected_time.tv_usec - st->usec_since_last_update;
		st->last_updated.tv_sec = (time_t) (ut / 1000000ULL);
		st->last_updated.tv_usec = (useconds_t) (ut % 1000000ULL);

		// the first entry should not be stored
		store_this_entry = 0;
		first_entry = 1;
	}

	// these are the 3 variables that will help us in interpolation
	// last_ut = the last time we added a value to the storage
	//  now_ut = the time the current value is taken at
	// next_ut = the time of the next interpolation point
	last_ut = st->last_updated.tv_sec * 1000000ULL + st->last_updated.tv_usec;
	now_ut  = st->last_collected_time.tv_sec * 1000000ULL + st->last_collected_time.tv_usec;
	next_ut = (st->last_updated.tv_sec + st->update_every) * 1000000ULL;

	if(unlikely(!first_entry && now_ut < next_ut)) {
		if(unlikely(st->debug)) debug(D_RRD_STATS, "%s: THIS IS IN THE SAME INTERPOLATION POINT", st->name);
	}

	if(unlikely(st->debug)) {
		debug(D_RRD_STATS, "%s: last ut = %0.3Lf (last updated time)", st->name, (long double)last_ut/1000000.0);
		debug(D_RRD_STATS, "%s: now  ut = %0.3Lf (current update time)", st->name, (long double)now_ut/1000000.0);
		debug(D_RRD_STATS, "%s: next ut = %0.3Lf (next interpolation point)", st->name, (long double)next_ut/1000000.0);
	}

	if(unlikely(!st->counter_done)) {
		store_this_entry = 0;
		if(unlikely(st->debug)) debug(D_RRD_STATS, "%s: Will not store the next entry.", st->name);
	}
	st->counter_done++;

	// calculate totals and count the dimensions
	int dimensions;
	st->collected_total = 0;
	for( rd = st->dimensions, dimensions = 0 ; likely(rd) ; rd = rd->next, dimensions++ )
		st->collected_total += rd->collected_value;

	uint32_t storage_flags = SN_EXISTS;

	// process all dimensions to calculate their values
	// based on the collected figures only
	// at this stage we do not interpolate anything
	for( rd = st->dimensions ; likely(rd) ; rd = rd->next ) {

		if(unlikely(st->debug)) debug(D_RRD_STATS, "%s/%s: START "
			" last_collected_value = " COLLECTED_NUMBER_FORMAT
			" collected_value = " COLLECTED_NUMBER_FORMAT
			" last_calculated_value = " CALCULATED_NUMBER_FORMAT
			" calculated_value = " CALCULATED_NUMBER_FORMAT
			, st->id, rd->name
			, rd->last_collected_value
			, rd->collected_value
			, rd->last_calculated_value
			, rd->calculated_value
			);

		switch(rd->algorithm) {
		case RRDDIM_ABSOLUTE:
			rd->calculated_value = (calculated_number)rd->collected_value
				* (calculated_number)rd->multiplier
				/ (calculated_number)rd->divisor;

			if(unlikely(st->debug))
				debug(D_RRD_STATS, "%s/%s: CALC ABS/ABS-NO-IN "
					CALCULATED_NUMBER_FORMAT " = "
					COLLECTED_NUMBER_FORMAT
					" * " CALCULATED_NUMBER_FORMAT
					" / " CALCULATED_NUMBER_FORMAT
					, st->id, rd->name
					, rd->calculated_value
					, rd->collected_value
					, (calculated_number)rd->multiplier
					, (calculated_number)rd->divisor
					);
			break;

			case RRDDIM_PCENT_OVER_ROW_TOTAL:
				if(unlikely(!st->collected_total)) rd->calculated_value = 0;
				else
				// the percentage of the current value
				// over the total of all dimensions
				rd->calculated_value =
					  (calculated_number)100
					* (calculated_number)rd->collected_value
					/ (calculated_number)st->collected_total;

				if(unlikely(st->debug))
					debug(D_RRD_STATS, "%s/%s: CALC PCENT-ROW "
						CALCULATED_NUMBER_FORMAT " = 100"
						" * " COLLECTED_NUMBER_FORMAT
						" / " COLLECTED_NUMBER_FORMAT
						, st->id, rd->name
						, rd->calculated_value
						, rd->collected_value
						, st->collected_total
						);
				break;

			case RRDDIM_INCREMENTAL:
				if(unlikely(!rd->updated || rd->counter <= 1)) {
					rd->calculated_value = 0;
					continue;
				}

				// if the new is smaller than the old (an overflow, or reset), set the old equal to the new
				// to reset the calculation (it will give zero as the calculation for this second)
				if(unlikely(rd->last_collected_value > rd->collected_value)) {
					debug(D_RRD_STATS, "%s.%s: RESET or OVERFLOW. Last collected value = " COLLECTED_NUMBER_FORMAT ", current = " COLLECTED_NUMBER_FORMAT
							, st->name, rd->name
							, rd->last_collected_value
							, rd->collected_value);
					if(!(rd->flags & RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS)) storage_flags = SN_EXISTS_RESET;
					rd->last_collected_value = rd->collected_value;
				}

				rd->calculated_value = (calculated_number)(rd->collected_value - rd->last_collected_value)
					* (calculated_number)rd->multiplier
					/ (calculated_number)rd->divisor;

				if(unlikely(st->debug))
					debug(D_RRD_STATS, "%s/%s: CALC INC PRE "
						CALCULATED_NUMBER_FORMAT " = ("
						COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT
						")"
						" * " CALCULATED_NUMBER_FORMAT
						" / " CALCULATED_NUMBER_FORMAT
						, st->id, rd->name
						, rd->calculated_value
						, rd->collected_value, rd->last_collected_value
						, (calculated_number)rd->multiplier
						, (calculated_number)rd->divisor
						);
				break;

			case RRDDIM_PCENT_OVER_DIFF_TOTAL:
				if(unlikely(!rd->updated || rd->counter <= 1)) {
					rd->calculated_value = 0;
					continue;
				}

				// the percentage of the current increment
				// over the increment of all dimensions together
				if(unlikely(st->collected_total == st->last_collected_total)) rd->calculated_value = rd->last_calculated_value;
				else rd->calculated_value =
					  (calculated_number)100
					* (calculated_number)(rd->collected_value - rd->last_collected_value)
					/ (calculated_number)(st->collected_total  - st->last_collected_total);

				if(unlikely(st->debug))
					debug(D_RRD_STATS, "%s/%s: CALC PCENT-DIFF "
						CALCULATED_NUMBER_FORMAT " = 100"
						" * (" COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT ")"
						" / (" COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT ")"
						, st->id, rd->name
						, rd->calculated_value
						, rd->collected_value, rd->last_collected_value
						, st->collected_total, st->last_collected_total
						);
				break;

			default:
				// make the default zero, to make sure
				// it gets noticed when we add new types
				rd->calculated_value = 0;

				if(unlikely(st->debug))
					debug(D_RRD_STATS, "%s/%s: CALC "
						CALCULATED_NUMBER_FORMAT " = 0"
						, st->id, rd->name
						, rd->calculated_value
						);
				break;
		}

		if(unlikely(st->debug)) debug(D_RRD_STATS, "%s/%s: PHASE2 "
			" last_collected_value = " COLLECTED_NUMBER_FORMAT
			" collected_value = " COLLECTED_NUMBER_FORMAT
			" last_calculated_value = " CALCULATED_NUMBER_FORMAT
			" calculated_value = " CALCULATED_NUMBER_FORMAT
			, st->id, rd->name
			, rd->last_collected_value
			, rd->collected_value
			, rd->last_calculated_value
			, rd->calculated_value
			);

	}

	// at this point we have all the calculated values ready
	// it is now time to interpolate values on a second boundary

	unsigned long long first_ut = last_ut;
	long long iterations = (now_ut - last_ut) / (st->update_every * 1000000ULL);
	if((now_ut % (st->update_every * 1000000ULL)) == 0) iterations++;

	for( ; likely(next_ut <= now_ut) ; next_ut += st->update_every * 1000000ULL, iterations-- ) {
#ifdef NETDATA_INTERNAL_CHECKS
		if(iterations < 0) { error("%s: iterations calculation wrapped! first_ut = %llu, last_ut = %llu, next_ut = %llu, now_ut = %llu", st->name, first_ut, last_ut, next_ut, now_ut); }
#endif

		if(unlikely(st->debug)) {
			debug(D_RRD_STATS, "%s: last ut = %0.3Lf (last updated time)", st->name, (long double)last_ut/1000000.0);
			debug(D_RRD_STATS, "%s: next ut = %0.3Lf (next interpolation point)", st->name, (long double)next_ut/1000000.0);
		}

		st->last_updated.tv_sec = (time_t) (next_ut / 1000000ULL);
		st->last_updated.tv_usec = 0;

		for( rd = st->dimensions ; likely(rd) ; rd = rd->next ) {
			calculated_number new_value;

			switch(rd->algorithm) {
				case RRDDIM_INCREMENTAL:
					new_value = (calculated_number)
						(	   rd->calculated_value
							* (calculated_number)(next_ut - last_ut)
							/ (calculated_number)(now_ut - last_ut)
						);

					if(unlikely(st->debug))
						debug(D_RRD_STATS, "%s/%s: CALC2 INC "
							CALCULATED_NUMBER_FORMAT " = "
							CALCULATED_NUMBER_FORMAT
							" * %llu"
							" / %llu"
							, st->id, rd->name
							, new_value
							, rd->calculated_value
							, (next_ut - last_ut)
							, (now_ut - last_ut)
							);

					rd->calculated_value -= new_value;
					new_value += rd->last_calculated_value;
					rd->last_calculated_value = 0;
					new_value /= (calculated_number)st->update_every;
					break;

				case RRDDIM_ABSOLUTE:
				case RRDDIM_PCENT_OVER_ROW_TOTAL:
				case RRDDIM_PCENT_OVER_DIFF_TOTAL:
				default:
					if(iterations == 1) {
						// this is the last iteration
						// do not interpolate
						// just show the calculated value

						new_value = rd->calculated_value;
					}
					else {
						// we have missed an update
						// interpolate in the middle values

						new_value = (calculated_number)
							(	(	  (rd->calculated_value - rd->last_calculated_value)
									* (calculated_number)(next_ut - first_ut)
									/ (calculated_number)(now_ut - first_ut)
								)
								+  rd->last_calculated_value
							);

						if(unlikely(st->debug))
							debug(D_RRD_STATS, "%s/%s: CALC2 DEF "
								CALCULATED_NUMBER_FORMAT " = ((("
								"(" CALCULATED_NUMBER_FORMAT " - " CALCULATED_NUMBER_FORMAT ")"
								" * %llu"
								" / %llu) + " CALCULATED_NUMBER_FORMAT
								, st->id, rd->name
								, new_value
								, rd->calculated_value, rd->last_calculated_value
								, (next_ut - first_ut)
								, (now_ut - first_ut), rd->last_calculated_value
								);

						// this is wrong
						// it fades the value towards the target
						// while we know the calculated value is different
						// if(likely(next_ut + st->update_every * 1000000ULL > now_ut)) rd->calculated_value = new_value;
					}
					break;
			}

			if(unlikely(!store_this_entry)) {
				store_this_entry = 1;
				continue;
			}

			if(likely(rd->updated && rd->counter > 1 && iterations < st->gap_when_lost_iterations_above)) {
				rd->values[st->current_entry] = pack_storage_number(new_value, storage_flags );

				if(unlikely(st->debug))
					debug(D_RRD_STATS, "%s/%s: STORE[%ld] "
						CALCULATED_NUMBER_FORMAT " = " CALCULATED_NUMBER_FORMAT
						, st->id, rd->name
						, st->current_entry
						, unpack_storage_number(rd->values[st->current_entry]), new_value
						);
			}
			else {
				if(unlikely(st->debug)) debug(D_RRD_STATS, "%s/%s: STORE[%ld] = NON EXISTING "
						, st->id, rd->name
						, st->current_entry
						);
				rd->values[st->current_entry] = pack_storage_number(0, SN_NOT_EXISTS);
			}

			stored_entries++;

			if(unlikely(st->debug)) {
				calculated_number t1 = new_value * (calculated_number)rd->multiplier / (calculated_number)rd->divisor;
				calculated_number t2 = unpack_storage_number(rd->values[st->current_entry]);
				calculated_number accuracy = accuracy_loss(t1, t2);
				debug(D_RRD_STATS, "%s/%s: UNPACK[%ld] = " CALCULATED_NUMBER_FORMAT " FLAGS=0x%08x (original = " CALCULATED_NUMBER_FORMAT ", accuracy loss = " CALCULATED_NUMBER_FORMAT "%%%s)"
						, st->id, rd->name
						, st->current_entry
						, t2
						, get_storage_number_flags(rd->values[st->current_entry])
						, t1
						, accuracy
						, (accuracy > ACCURACY_LOSS) ? " **TOO BIG** " : ""
						);

				rd->collected_volume += t1;
				rd->stored_volume += t2;
				accuracy = accuracy_loss(rd->collected_volume, rd->stored_volume);
				debug(D_RRD_STATS, "%s/%s: VOLUME[%ld] = " CALCULATED_NUMBER_FORMAT ", calculated  = " CALCULATED_NUMBER_FORMAT ", accuracy loss = " CALCULATED_NUMBER_FORMAT "%%%s"
						, st->id, rd->name
						, st->current_entry
						, rd->stored_volume
						, rd->collected_volume
						, accuracy
						, (accuracy > ACCURACY_LOSS) ? " **TOO BIG** " : ""
						);

			}
		}
		// reset the storage flags for the next point, if any;
		storage_flags = SN_EXISTS;

		st->counter++;
		st->current_entry = ((st->current_entry + 1) >= st->entries) ? 0 : st->current_entry + 1;
		last_ut = next_ut;
	}

	// align next interpolation to last collection point
	if(likely(stored_entries || !store_this_entry)) {
		st->last_updated.tv_sec = st->last_collected_time.tv_sec;
		st->last_updated.tv_usec = st->last_collected_time.tv_usec;
	}

	for( rd = st->dimensions; likely(rd) ; rd = rd->next ) {
		if(unlikely(!rd->updated)) continue;

		if(likely(stored_entries || !store_this_entry)) {
			if(unlikely(st->debug)) debug(D_RRD_STATS, "%s/%s: setting last_collected_value (old: " COLLECTED_NUMBER_FORMAT ") to last_collected_value (new: " COLLECTED_NUMBER_FORMAT ")", st->id, rd->name, rd->last_collected_value, rd->collected_value);
			rd->last_collected_value = rd->collected_value;

			if(unlikely(st->debug)) debug(D_RRD_STATS, "%s/%s: setting last_calculated_value (old: " CALCULATED_NUMBER_FORMAT ") to last_calculated_value (new: " CALCULATED_NUMBER_FORMAT ")", st->id, rd->name, rd->last_calculated_value, rd->calculated_value);
			rd->last_calculated_value = rd->calculated_value;
		}

		rd->calculated_value = 0;
		rd->collected_value = 0;
		rd->updated = 0;

		if(unlikely(st->debug)) debug(D_RRD_STATS, "%s/%s: END "
			" last_collected_value = " COLLECTED_NUMBER_FORMAT
			" collected_value = " COLLECTED_NUMBER_FORMAT
			" last_calculated_value = " CALCULATED_NUMBER_FORMAT
			" calculated_value = " CALCULATED_NUMBER_FORMAT
			, st->id, rd->name
			, rd->last_collected_value
			, rd->collected_value
			, rd->last_calculated_value
			, rd->calculated_value
			);
	}
	st->last_collected_total  = st->collected_total;

	// ALL DONE ABOUT THE DATA UPDATE
	// --------------------------------------------------------------------

	// find if there are any obsolete dimensions (not updated recently)
	if(unlikely(rrd_delete_unupdated_dimensions)) {

		for( rd = st->dimensions; likely(rd) ; rd = rd->next )
			if((rd->last_collected_time.tv_sec + (rrd_delete_unupdated_dimensions * st->update_every)) < st->last_collected_time.tv_sec)
				break;

		if(unlikely(rd)) {
			// there is dimension to free
			// upgrade our read lock to a write lock
			pthread_rwlock_unlock(&st->rwlock);
			pthread_rwlock_wrlock(&st->rwlock);

			for( rd = st->dimensions, last = NULL ; likely(rd) ; ) {
				// remove it only it is not updated in rrd_delete_unupdated_dimensions seconds

				if(unlikely((rd->last_collected_time.tv_sec + (rrd_delete_unupdated_dimensions * st->update_every)) < st->last_collected_time.tv_sec)) {
					info("Removing obsolete dimension '%s' (%s) of '%s' (%s).", rd->name, rd->id, st->name, st->id);

					if(unlikely(!last)) {
						st->dimensions = rd->next;
						rd->next = NULL;
						rrddim_free(st, rd);
						rd = st->dimensions;
						continue;
					}
					else {
						last->next = rd->next;
						rd->next = NULL;
						rrddim_free(st, rd);
						rd = last->next;
						continue;
					}
				}

				last = rd;
				rd = rd->next;
			}

			if(unlikely(!st->dimensions)) {
				info("Disabling chart %s (%s) since it does not have any dimensions", st->name, st->id);
				st->enabled = 0;
			}
		}
	}

	pthread_rwlock_unlock(&st->rwlock);

	if(unlikely(pthread_setcancelstate(oldstate, NULL) != 0))
		error("Cannot set pthread cancel state to RESTORE (%d).", oldstate);

	return(st->usec_since_last_update);
}
