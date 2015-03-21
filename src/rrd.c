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

#include "log.h"
#include "config.h"
#include "common.h"

#include "rrd.h"

int update_every = UPDATE_EVERY;
int save_history = HISTORY;

// ----------------------------------------------------------------------------
// chart types

int chart_type_id(const char *name)
{
	if(strcmp(name, CHART_TYPE_AREA_NAME) == 0) return CHART_TYPE_AREA;
	if(strcmp(name, CHART_TYPE_STACKED_NAME) == 0) return CHART_TYPE_STACKED;
	if(strcmp(name, CHART_TYPE_LINE_NAME) == 0) return CHART_TYPE_LINE;
	return CHART_TYPE_LINE;
}

const char *chart_type_name(int chart_type)
{
	static char line[] = CHART_TYPE_LINE_NAME;
	static char area[] = CHART_TYPE_AREA_NAME;
	static char stacked[] = CHART_TYPE_STACKED_NAME;

	switch(chart_type) {
		case CHART_TYPE_LINE:
			return line;

		case CHART_TYPE_AREA:
			return area;

		case CHART_TYPE_STACKED:
			return stacked;
	}
	return line;
}

// ----------------------------------------------------------------------------
// mmap() wrapper

int memory_mode = NETDATA_MEMORY_MODE_SAVE;

const char *memory_mode_name(int id)
{
	static const char ram[] = NETDATA_MEMORY_MODE_RAM_NAME;
	static const char map[] = NETDATA_MEMORY_MODE_MAP_NAME;
	static const char save[] = NETDATA_MEMORY_MODE_SAVE_NAME;

	switch(id) {
		case NETDATA_MEMORY_MODE_RAM:
			return ram;

		case NETDATA_MEMORY_MODE_MAP:
			return map;

		case NETDATA_MEMORY_MODE_SAVE:
		default:
			return save;
	}

	return save;
}

int memory_mode_id(const char *name)
{
	if(!strcmp(name, NETDATA_MEMORY_MODE_RAM_NAME))
		return NETDATA_MEMORY_MODE_RAM;
	else if(!strcmp(name, NETDATA_MEMORY_MODE_MAP_NAME))
		return NETDATA_MEMORY_MODE_MAP;

	return NETDATA_MEMORY_MODE_SAVE;
}

// ----------------------------------------------------------------------------
// algorithms types

int algorithm_id(const char *name)
{
	if(strcmp(name, RRD_DIMENSION_ABSOLUTE_NAME) == 0) 			return RRD_DIMENSION_ABSOLUTE;
	if(strcmp(name, RRD_DIMENSION_INCREMENTAL_NAME) == 0) 			return RRD_DIMENSION_INCREMENTAL;
	if(strcmp(name, RRD_DIMENSION_PCENT_OVER_ROW_TOTAL_NAME) == 0) 		return RRD_DIMENSION_PCENT_OVER_ROW_TOTAL;
	if(strcmp(name, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL_NAME) == 0) 	return RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL;
	return RRD_DIMENSION_ABSOLUTE;
}

const char *algorithm_name(int chart_type)
{
	static char absolute[] = RRD_DIMENSION_ABSOLUTE_NAME;
	static char incremental[] = RRD_DIMENSION_INCREMENTAL_NAME;
	static char percentage_of_absolute_row[] = RRD_DIMENSION_PCENT_OVER_ROW_TOTAL_NAME;
	static char percentage_of_incremental_row[] = RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL_NAME;

	switch(chart_type) {
		case RRD_DIMENSION_ABSOLUTE:
			return absolute;

		case RRD_DIMENSION_INCREMENTAL:
			return incremental;

		case RRD_DIMENSION_PCENT_OVER_ROW_TOTAL:
			return percentage_of_absolute_row;

		case RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL:
			return percentage_of_incremental_row;
	}
	return absolute;
}

RRD_STATS *root = NULL;
pthread_rwlock_t root_rwlock = PTHREAD_RWLOCK_INITIALIZER;

char *rrd_stats_strncpy_name(char *to, const char *from, int length)
{
	int i;
	for(i = 0; i < length && from[i] ;i++) {
		if(from[i] == '.' || isalpha(from[i]) || isdigit(from[i])) to[i] = from[i];
		else to[i] = '_';
	}
	if(i < length) to[i] = '\0';
	to[length - 1] = '\0';

	return to;
}

void rrd_stats_set_name(RRD_STATS *st, const char *name)
{
	char b[CONFIG_MAX_VALUE + 1];
	char n[RRD_STATS_NAME_MAX + 1];

	snprintf(n, RRD_STATS_NAME_MAX, "%s.%s", st->type, name);
	rrd_stats_strncpy_name(b, n, CONFIG_MAX_VALUE);
	st->name = config_get(st->id, "name", b);
	st->hash_name = simple_hash(st->name);
}

char *rrd_stats_cache_dir(const char *id)
{
	char *ret = NULL;

	static char *cache_dir = NULL;
	if(!cache_dir) cache_dir = config_get("global", "database directory", "cache");

	char b[FILENAME_MAX + 1];
	char n[FILENAME_MAX + 1];
	rrd_stats_strncpy_name(b, id, FILENAME_MAX);

	snprintf(n, FILENAME_MAX, "%s/%s", cache_dir, b);
	ret = config_get(id, "database directory", n);

	if(memory_mode == NETDATA_MEMORY_MODE_MAP || memory_mode == NETDATA_MEMORY_MODE_SAVE) {
		int r = mkdir(ret, 0775);
		if(r != 0 && errno != EEXIST)
			error("Cannot create directory '%s'", ret);
	}

	return ret;
}

void rrd_stats_reset(RRD_STATS *st)
{
	st->last_collected_time.tv_sec = 0;
	st->last_collected_time.tv_usec = 0;
	st->last_updated.tv_sec = 0;
	st->last_updated.tv_usec = 0;
	st->current_entry = 0;
	st->counter = 0;
	st->counter_done = 0;

	RRD_DIMENSION *rd;
	for(rd = st->dimensions; rd ; rd = rd->next) {
		rd->last_collected_time.tv_sec = 0;
		rd->last_collected_time.tv_usec = 0;
		bzero(rd->values, rd->entries * sizeof(storage_number));
	}
}

RRD_STATS *rrd_stats_create(const char *type, const char *id, const char *name, const char *family, const char *title, const char *units, long priority, int update_every, int chart_type)
{
	if(!id || !id[0]) {
		fatal("Cannot create rrd stats without an id.");
		return NULL;
	}

	char fullid[RRD_STATS_NAME_MAX + 1];
	char fullfilename[FILENAME_MAX + 1];
	RRD_STATS *st = NULL;

	snprintf(fullid, RRD_STATS_NAME_MAX, "%s.%s", type, id);

	long entries = config_get_number(fullid, "history", save_history);
	if(entries < 5) entries = config_set_number(fullid, "history", 5);
	if(entries > HISTORY_MAX) entries = config_set_number(fullid, "history", HISTORY_MAX);

	int enabled = config_get_boolean(fullid, "enabled", 1);
	if(!enabled) entries = 5;

	unsigned long size = sizeof(RRD_STATS);
	char *cache_dir = rrd_stats_cache_dir(fullid);

	debug(D_RRD_CALLS, "Creating RRD_STATS for '%s.%s'.", type, id);

	snprintf(fullfilename, FILENAME_MAX, "%s/main.db", cache_dir);
	if(memory_mode != NETDATA_MEMORY_MODE_RAM) st = (RRD_STATS *)mymmap(fullfilename, size, ((memory_mode == NETDATA_MEMORY_MODE_MAP)?MAP_SHARED:MAP_PRIVATE));
	if(st) {
		if(strcmp(st->magic, RRD_STATS_MAGIC) != 0) {
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
		st->title = NULL;
		st->units = NULL;
		st->dimensions = NULL;
		st->next = NULL;
		st->mapped = memory_mode;
	}
	else {
		st = calloc(1, size);
		if(!st) {
			fatal("Cannot allocate memory for RRD_STATS %s.%s", type, id);
			return NULL;
		}
		st->mapped = NETDATA_MEMORY_MODE_RAM;
	}
	st->memsize = size;
	st->entries = entries;
	st->update_every = update_every;

	strcpy(st->cache_file, fullfilename);
	strcpy(st->magic, RRD_STATS_MAGIC);

	strcpy(st->id, fullid);
	st->hash = simple_hash(st->id);

	st->cache_dir = cache_dir;

	st->family     = config_get(st->id, "family", family?family:st->id);
	st->units      = config_get(st->id, "units", units?units:"");
	st->type       = config_get(st->id, "type", type);
	st->chart_type = chart_type_id(config_get(st->id, "chart type", chart_type_name(chart_type)));

	if(name && *name) rrd_stats_set_name(st, name);
	else rrd_stats_set_name(st, id);

	{
		char varvalue[CONFIG_MAX_VALUE + 1];
		snprintf(varvalue, CONFIG_MAX_VALUE, "%s (%s)", title?title:"", st->name);
		st->title = config_get(st->id, "title", varvalue);
	}

	st->priority = config_get_number(st->id, "priority", priority);
	st->enabled = enabled;
	
	st->isdetail = 0;
	st->debug = 0;

	st->last_collected_time.tv_sec = 0;
	st->last_collected_time.tv_usec = 0;
	st->counter_done = 0;

	st->gap_when_lost_iterations = config_get_number(st->id, "gap when lost iterations above", DEFAULT_GAP_INTERPOLATIONS);

	pthread_rwlock_init(&st->rwlock, NULL);
	pthread_rwlock_wrlock(&root_rwlock);

	st->next = root;
	root = st;

	pthread_rwlock_unlock(&root_rwlock);

	return(st);
}

RRD_DIMENSION *rrd_stats_dimension_add(RRD_STATS *st, const char *id, const char *name, long multiplier, long divisor, int algorithm)
{
	char filename[FILENAME_MAX + 1];
	char fullfilename[FILENAME_MAX + 1];

	char varname[CONFIG_MAX_NAME + 1];
	RRD_DIMENSION *rd = NULL;
	unsigned long size = sizeof(RRD_DIMENSION) + (st->entries * sizeof(storage_number));

	debug(D_RRD_CALLS, "Adding dimension '%s/%s'.", st->id, id);

	rrd_stats_strncpy_name(filename, id, FILENAME_MAX);
	snprintf(fullfilename, FILENAME_MAX, "%s/%s.db", st->cache_dir, filename);
	if(memory_mode != NETDATA_MEMORY_MODE_RAM) rd = (RRD_DIMENSION *)mymmap(fullfilename, size, ((memory_mode == NETDATA_MEMORY_MODE_MAP)?MAP_SHARED:MAP_PRIVATE));
	if(rd) {
		struct timeval now;
		gettimeofday(&now, NULL);

		if(strcmp(rd->magic, RRD_DIMENSION_MAGIC) != 0) {
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
		rd->mapped = memory_mode;
		rd->hidden = 0;
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

		rd->mapped = NETDATA_MEMORY_MODE_RAM;
	}
	rd->memsize = size;

	strcpy(rd->magic, RRD_DIMENSION_MAGIC);
	strcpy(rd->cache_file, fullfilename);
	strncpy(rd->id, id, RRD_STATS_NAME_MAX);
	rd->hash = simple_hash(rd->id);

	snprintf(varname, CONFIG_MAX_NAME, "dim %s name", rd->id);
	rd->name = config_get(st->id, varname, (name && *name)?name:rd->id);

	snprintf(varname, CONFIG_MAX_NAME, "dim %s algorithm", rd->id);
	rd->algorithm = algorithm_id(config_get(st->id, varname, algorithm_name(algorithm)));

	snprintf(varname, CONFIG_MAX_NAME, "dim %s multiplier", rd->id);
	rd->multiplier = config_get_number(st->id, varname, multiplier);

	snprintf(varname, CONFIG_MAX_NAME, "dim %s divisor", rd->id);
	rd->divisor = config_get_number(st->id, varname, divisor);
	if(!rd->divisor) rd->divisor = 1;

	rd->entries = st->entries;
	rd->update_every = st->update_every;
	
	// append this dimension
	if(!st->dimensions)
		st->dimensions = rd;
	else {
		RRD_DIMENSION *td = st->dimensions;
		for(; td->next; td = td->next) ;
		td->next = rd;
	}

	return(rd);
}

void rrd_stats_dimension_set_name(RRD_STATS *st, RRD_DIMENSION *rd, const char *name)
{
	char varname[CONFIG_MAX_NAME + 1];
	snprintf(varname, CONFIG_MAX_NAME, "dim %s name", rd->id);
	config_get(st->id, varname, name);
}

void rrd_stats_dimension_free(RRD_DIMENSION *rd)
{
	if(rd->next) rrd_stats_dimension_free(rd->next);
	// free(rd->annotations);
	if(rd->mapped == NETDATA_MEMORY_MODE_SAVE) {
		debug(D_RRD_CALLS, "Saving dimension '%s' to '%s'.", rd->name, rd->cache_file);
		savememory(rd->cache_file, rd, rd->memsize);

		debug(D_RRD_CALLS, "Unmapping dimension '%s'.", rd->name);
		munmap(rd, rd->memsize);
	}
	else if(rd->mapped == NETDATA_MEMORY_MODE_MAP) {
		debug(D_RRD_CALLS, "Unmapping dimension '%s'.", rd->name);
		munmap(rd, rd->memsize);
	}
	else {
		debug(D_RRD_CALLS, "Removing dimension '%s'.", rd->name);
		free(rd);
	}
}

void rrd_stats_free_all(void)
{
	info("Freeing all memory...");

	RRD_STATS *st;
	for(st = root; st ;) {
		RRD_STATS *next = st->next;

		if(st->dimensions) rrd_stats_dimension_free(st->dimensions);
		st->dimensions = NULL;

		if(st->mapped == NETDATA_MEMORY_MODE_SAVE) {
			debug(D_RRD_CALLS, "Saving stats '%s' to '%s'.", st->name, st->cache_file);
			savememory(st->cache_file, st, st->memsize);

			debug(D_RRD_CALLS, "Unmapping stats '%s'.", st->name);
			munmap(st, st->memsize);
		}
		else if(st->mapped == NETDATA_MEMORY_MODE_MAP) {
			debug(D_RRD_CALLS, "Unmapping stats '%s'.", st->name);
			munmap(st, st->memsize);
		}
		else
			free(st);

		st = next;
	}
	root = NULL;

	info("Memory cleanup completed...");
}

void rrd_stats_save_all(void)
{
	RRD_STATS *st;
	RRD_DIMENSION *rd;

	pthread_rwlock_wrlock(&root_rwlock);
	for(st = root; st ; st = st->next) {
		pthread_rwlock_wrlock(&st->rwlock);

		if(st->mapped == NETDATA_MEMORY_MODE_SAVE) {
			debug(D_RRD_CALLS, "Saving stats '%s' to '%s'.", st->name, st->cache_file);
			savememory(st->cache_file, st, st->memsize);
		}

		for(rd = st->dimensions; rd ; rd = rd->next) {
			if(rd->mapped == NETDATA_MEMORY_MODE_SAVE) {
				debug(D_RRD_CALLS, "Saving dimension '%s' to '%s'.", rd->name, rd->cache_file);
				savememory(rd->cache_file, rd, rd->memsize);
			}
		}

		pthread_rwlock_unlock(&st->rwlock);
	}
	pthread_rwlock_unlock(&root_rwlock);
}


RRD_STATS *rrd_stats_find(const char *id)
{
	debug(D_RRD_CALLS, "rrd_stats_find() for chart %s", id);

	unsigned long hash = simple_hash(id);

	pthread_rwlock_rdlock(&root_rwlock);
	RRD_STATS *st = root;
	for ( ; st ; st = st->next )
		if(hash == st->hash)
			if(strcmp(st->id, id) == 0)
				break;
	pthread_rwlock_unlock(&root_rwlock);

	return(st);
}

RRD_STATS *rrd_stats_find_bytype(const char *type, const char *id)
{
	debug(D_RRD_CALLS, "rrd_stats_find_bytype() for chart %s.%s", type, id);

	char buf[RRD_STATS_NAME_MAX + 1];

	strncpy(buf, type, RRD_STATS_NAME_MAX - 1);
	buf[RRD_STATS_NAME_MAX - 1] = '\0';
	strcat(buf, ".");
	int len = strlen(buf);
	strncpy(&buf[len], id, RRD_STATS_NAME_MAX - len);
	buf[RRD_STATS_NAME_MAX] = '\0';

	return(rrd_stats_find(buf));
}

RRD_STATS *rrd_stats_find_byname(const char *name)
{
	debug(D_RRD_CALLS, "rrd_stats_find_byname() for chart %s", name);

	char b[CONFIG_MAX_VALUE + 1];

	rrd_stats_strncpy_name(b, name, CONFIG_MAX_VALUE);
	unsigned long hash = simple_hash(b);

	pthread_rwlock_rdlock(&root_rwlock);
	RRD_STATS *st = root;
	for ( ; st ; st = st->next ) {
		if(hash == st->hash_name && strcmp(st->name, b) == 0) break;
	}
	pthread_rwlock_unlock(&root_rwlock);

	return(st);
}

RRD_DIMENSION *rrd_stats_dimension_find(RRD_STATS *st, const char *id)
{
	debug(D_RRD_CALLS, "rrd_stats_dimension_find() for chart %s, dimension %s", st->name, id);

	unsigned long hash = simple_hash(id);

	RRD_DIMENSION *rd = st->dimensions;

	for ( ; rd ; rd = rd->next )
		if(hash == rd->hash)
			if(strcmp(rd->id, id) == 0)
				break;

	return(rd);
}

int rrd_stats_dimension_hide(RRD_STATS *st, const char *id)
{
	debug(D_RRD_CALLS, "rrd_stats_dimension_hide() for chart %s, dimension %s", st->name, id);

	RRD_DIMENSION *rd = rrd_stats_dimension_find(st, id);
	if(!rd) {
		error("Cannot find dimension with id '%s' on stats '%s' (%s).", id, st->name, st->id);
		return 1;
	}

	rd->hidden = 1;
	return 0;
}

void rrd_stats_dimension_set_by_pointer(RRD_STATS *st, RRD_DIMENSION *rd, collected_number value)
{
	debug(D_RRD_CALLS, "rrd_stats_dimension_set() for chart %s, dimension %s, value " COLLECTED_NUMBER_FORMAT, st->name, rd->name, value);
	
	gettimeofday(&rd->last_collected_time, NULL);
	rd->collected_value = value;
	rd->updated = 1;
}

int rrd_stats_dimension_set(RRD_STATS *st, const char *id, collected_number value)
{
	RRD_DIMENSION *rd = rrd_stats_dimension_find(st, id);
	if(!rd) {
		error("Cannot find dimension with id '%s' on stats '%s' (%s).", id, st->name, st->id);
		return 1;
	}

	rrd_stats_dimension_set_by_pointer(st, rd, value);
	return 0;
}

void rrd_stats_next_usec(RRD_STATS *st, unsigned long long microseconds)
{
	debug(D_RRD_CALLS, "rrd_stats_next() for chart %s with microseconds %llu", st->name, microseconds);

	if(st->debug) debug(D_RRD_STATS, "%s: NEXT: %llu microseconds", st->name, microseconds);
	st->usec_since_last_update = microseconds;
}

void rrd_stats_next(RRD_STATS *st)
{
	unsigned long long microseconds = 0;

	if(st->last_collected_time.tv_sec) {
		struct timeval now;
		gettimeofday(&now, NULL);
		microseconds = usecdiff(&now, &st->last_collected_time);
	}

	rrd_stats_next_usec(st, microseconds);
}

void rrd_stats_next_plugins(RRD_STATS *st)
{
	rrd_stats_next(st);
}

unsigned long long rrd_stats_done(RRD_STATS *st)
{
	debug(D_RRD_CALLS, "rrd_stats_done() for chart %s", st->name);

	RRD_DIMENSION *rd, *last;
	int oldstate, store_this_entry = 1;

	if(pthread_setcancelstate(PTHREAD_CANCEL_DISABLE, &oldstate) != 0)
		error("Cannot set pthread cancel state to DISABLE.");

	// a read lock is OK here
	pthread_rwlock_rdlock(&st->rwlock);

	// check if the chart has a long time to be refreshed
	if(st->usec_since_last_update > st->entries * st->update_every * 1000000ULL) {
		info("%s: took too long to be updated (%0.3Lf secs). Reseting it.", st->name, (long double)(st->usec_since_last_update / 1000000.0));
		rrd_stats_reset(st);
		st->usec_since_last_update = st->update_every * 1000000ULL;
	}
	if(st->debug) debug(D_RRD_STATS, "%s: microseconds since last update: %llu", st->name, st->usec_since_last_update);

	// set last_collected_time
	if(!st->last_collected_time.tv_sec) {
		// it is the first entry
		// set the last_collected_time to now
		gettimeofday(&st->last_collected_time, NULL);

		// the first entry should not be stored
		store_this_entry = 0;

		if(st->debug) debug(D_RRD_STATS, "%s: initializing last_collected to now. Will not store the next entry.", st->name);
	}
	else {
		// it is not the first entry
		// calculate the proper last_collected_time, using usec_since_last_update
		unsigned long long ut = st->last_collected_time.tv_sec * 1000000ULL + st->last_collected_time.tv_usec + st->usec_since_last_update;
		st->last_collected_time.tv_sec = ut / 1000000ULL;
		st->last_collected_time.tv_usec = ut % 1000000ULL;
	}

	// if this set has not been updated in the past
	// we fake the last_update time to be = now - usec_since_last_update
	if(!st->last_updated.tv_sec) {
		// it has never been updated before
		// set a fake last_updated, in the past using usec_since_last_update
		unsigned long long ut = st->last_collected_time.tv_sec * 1000000ULL + st->last_collected_time.tv_usec - st->usec_since_last_update;
		st->last_updated.tv_sec = ut / 1000000ULL;
		st->last_updated.tv_usec = ut % 1000000ULL;

		// the first entry should not be stored
		store_this_entry = 0;

		if(st->debug) debug(D_RRD_STATS, "%s: initializing last_updated to now - %llu microseconds (%0.3Lf). Will not store the next entry.", st->name, st->usec_since_last_update, (long double)ut/1000000.0);
	}

	// check if we will re-write the entire data set
	if(usecdiff(&st->last_collected_time, &st->last_updated) > st->update_every * st->entries * 1000000ULL) {
		info("%s: too old data (last updated at %u.%u, last collected at %u.%u). Reseting it. Will not store the next entry.", st->name, st->last_updated.tv_sec, st->last_updated.tv_usec, st->last_collected_time.tv_sec, st->last_collected_time.tv_usec);
		rrd_stats_reset(st);

		st->usec_since_last_update = st->update_every * 1000000ULL;

		gettimeofday(&st->last_collected_time, NULL);

		unsigned long long ut = st->last_collected_time.tv_sec * 1000000ULL + st->last_collected_time.tv_usec - st->usec_since_last_update;
		st->last_updated.tv_sec = ut / 1000000ULL;
		st->last_updated.tv_usec = ut % 1000000ULL;

		// the first entry should not be stored
		store_this_entry = 0;
	}

	// these are the 3 variables that will help us in interpolation
	// last_ut = the last time we added a value to the storage
	//  now_ut = the time the current value is taken at
	// next_ut = the time of the next interpolation point
	unsigned long long last_ut = st->last_updated.tv_sec * 1000000ULL + st->last_updated.tv_usec;
	unsigned long long now_ut  = st->last_collected_time.tv_sec * 1000000ULL + st->last_collected_time.tv_usec;
	unsigned long long next_ut = (st->last_updated.tv_sec + st->update_every) * 1000000ULL;

	if(st->debug) debug(D_RRD_STATS, "%s: last ut = %0.3Lf (last updated time)", st->name, (long double)last_ut/1000000.0);
	if(st->debug) debug(D_RRD_STATS, "%s: now  ut = %0.3Lf (current update time)", st->name, (long double)now_ut/1000000.0);
	if(st->debug) debug(D_RRD_STATS, "%s: next ut = %0.3Lf (next interpolation point)", st->name, (long double)next_ut/1000000.0);

	if(!st->counter_done) {
		store_this_entry = 0;
		if(st->debug) debug(D_RRD_STATS, "%s: Will not store the next entry.", st->name);
	}
	st->counter_done++;

	// calculate totals and count the dimensions
	int dimensions;
	st->collected_total = 0;
	for( rd = st->dimensions, dimensions = 0 ; rd ; rd = rd->next, dimensions++ )
		st->collected_total += rd->collected_value;

	uint32_t storage_flags = SN_EXISTS;

	// process all dimensions to calculate their values
	// based on the collected figures only
	// at this stage we do not interpolate anything
	for( rd = st->dimensions ; rd ; rd = rd->next ) {

		if(st->debug) debug(D_RRD_STATS, "%s/%s: "
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
			case RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL:
				// the percentage of the current increment
				// over the increment of all dimensions together
				if(st->collected_total == st->last_collected_total) rd->calculated_value = rd->last_calculated_value;
				else rd->calculated_value =
					  (calculated_number)100
					* (calculated_number)(rd->collected_value - rd->last_collected_value)
					/ (calculated_number)(st->collected_total  - st->last_collected_total);

				if(st->debug)
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

			case RRD_DIMENSION_PCENT_OVER_ROW_TOTAL:
				if(!st->collected_total) rd->calculated_value = 0;
				else
				// the percentage of the current value
				// over the total of all dimensions
				rd->calculated_value =
					  (calculated_number)100
					* (calculated_number)rd->collected_value
					/ (calculated_number)st->collected_total;

				if(st->debug)
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

			case RRD_DIMENSION_INCREMENTAL:
				// if the new is smaller than the old (an overflow, or reset), set the old equal to the new
				// to reset the calculation (it will give zero as the calculation for this second)
				if(rd->last_collected_value > rd->collected_value) {
					storage_flags = SN_EXISTS_RESET;
					rd->last_collected_value = rd->collected_value;
				}

				rd->calculated_value += (calculated_number)(rd->collected_value - rd->last_collected_value);

				if(st->debug)
					debug(D_RRD_STATS, "%s/%s: CALC INC "
						CALCULATED_NUMBER_FORMAT " += "
						COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT
						, st->id, rd->name
						, rd->calculated_value
						, rd->collected_value, rd->last_collected_value
						);
				break;

			case RRD_DIMENSION_ABSOLUTE:
				rd->calculated_value = (calculated_number)rd->collected_value;

				if(st->debug)
					debug(D_RRD_STATS, "%s/%s: CALC ABS/ABS-NO-IN "
						CALCULATED_NUMBER_FORMAT " = "
						COLLECTED_NUMBER_FORMAT
						, st->id, rd->name
						, rd->calculated_value
						, rd->collected_value
						);
				break;

			default:
				// make the default zero, to make sure
				// it gets noticed when we add new types
				rd->calculated_value = 0;

				if(st->debug)
					debug(D_RRD_STATS, "%s/%s: CALC "
						CALCULATED_NUMBER_FORMAT " = 0"
						, st->id, rd->name
						, rd->calculated_value
						);
				break;
		}
	}

	// at this point we have all the calculated values ready
	// it is now time to interpolate values on a second boundary

	unsigned long long first_ut = last_ut;
	int iterations = (now_ut - last_ut) / (st->update_every * 1000000ULL);

	for( ; next_ut <= now_ut ; next_ut += st->update_every * 1000000ULL, iterations-- ) {
		if(iterations < 0) error("iterations calculation wrapped!");

		if(st->debug) debug(D_RRD_STATS, "%s: last ut = %0.3Lf (last updated time)", st->name, (long double)last_ut/1000000.0);
		if(st->debug) debug(D_RRD_STATS, "%s: next ut = %0.3Lf (next interpolation point)", st->name, (long double)next_ut/1000000.0);

		st->last_updated.tv_sec = next_ut / 1000000ULL;
		st->last_updated.tv_usec = 0;

		for( rd = st->dimensions ; rd ; rd = rd->next ) {
			calculated_number new_value;

			switch(rd->algorithm) {
				case RRD_DIMENSION_INCREMENTAL:
					new_value = (calculated_number)
						(	   rd->calculated_value
							* (calculated_number)(next_ut - last_ut)
							/ (calculated_number)(now_ut - last_ut)
						);

					if(st->debug)
						debug(D_RRD_STATS, "%s/%s: CALC2 INC "
							CALCULATED_NUMBER_FORMAT " = "
							CALCULATED_NUMBER_FORMAT
							" * %llu"
							" / %llu"
							, st->id, rd->name
							, new_value
							, rd->calculated_value
							, (unsigned long long)(next_ut - last_ut)
							, (unsigned long long)(now_ut - last_ut)
							);

					rd->calculated_value -= new_value;
					break;

				case RRD_DIMENSION_ABSOLUTE:
				case RRD_DIMENSION_PCENT_OVER_ROW_TOTAL:
				case RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL:
				default:
					new_value = (calculated_number)
						(	(	  (rd->calculated_value - rd->last_calculated_value)
								* (calculated_number)(next_ut - first_ut)
								/ (calculated_number)(now_ut - first_ut)
							)
							+  rd->last_calculated_value
						);

					if(st->debug)
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

					if(next_ut + st->update_every * 1000000ULL > now_ut) rd->calculated_value = new_value;
					break;
			}

			if(!store_this_entry) {
				store_this_entry = 1;
				continue;
			}

			if(rd->updated && iterations < st->gap_when_lost_iterations) {
				rd->values[st->current_entry] = pack_storage_number(
						  new_value
						* (calculated_number)rd->multiplier
						/ (calculated_number)rd->divisor
					, storage_flags );

				if(st->debug)
					debug(D_RRD_STATS, "%s/%s: STORE[%ld] "
						CALCULATED_NUMBER_FORMAT " = " CALCULATED_NUMBER_FORMAT
						" * %ld"
						" / %ld"
						, st->id, rd->name
						, st->current_entry
						, unpack_storage_number(rd->values[st->current_entry]), new_value
						, rd->multiplier
						, rd->divisor
						);
			}
			else {
				if(st->debug) debug(D_RRD_STATS, "%s/%s: STORE[%ld] = NON EXISTING "
						, st->id, rd->name
						, st->current_entry
						);
				rd->values[st->current_entry] = pack_storage_number(0, SN_NOT_EXISTS);
			}

			if(st->debug) {
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

		if(st->first_entry_t && st->counter >= (unsigned long long)st->entries) {
			// the db is overwriting values
			// add the value we will overwrite
			st->first_entry_t += st->update_every * 1000000ULL;
		}
		
		st->counter++;
		st->current_entry = ((st->current_entry + 1) >= st->entries) ? 0 : st->current_entry + 1;
		if(!st->first_entry_t) st->first_entry_t = next_ut;
		last_ut = next_ut;
	}

	for( rd = st->dimensions; rd ; rd = rd->next ) {
		if(!rd->updated) continue;
		rd->last_collected_value = rd->collected_value;
		rd->last_calculated_value = rd->calculated_value;
		rd->collected_value = 0;
		rd->updated = 0;

		// if this is the first entry of incremental dimensions
		// we have to set the first calculated_value to zero
		// to eliminate the first spike
		if(st->counter_done == 1) switch(rd->algorithm) {
			case RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL:
			case RRD_DIMENSION_INCREMENTAL:
				rd->calculated_value = 0;
				// the next time, a new incremental total will be calculated
				break;
		}
	}
	st->last_collected_total  = st->collected_total;

	// ALL DONE ABOUT THE DATA UPDATE
	// --------------------------------------------------------------------

	// find if there are any obsolete dimensions (not updated recently)
	for( rd = st->dimensions; rd ; rd = rd->next )
		if((rd->last_collected_time.tv_sec + (10 * st->update_every)) < st->last_collected_time.tv_sec)
			break;

	if(rd) {
		// there is dimension to free
		// upgrade our read lock to a write lock
		pthread_rwlock_unlock(&st->rwlock);
		pthread_rwlock_wrlock(&st->rwlock);

		for( rd = st->dimensions, last = NULL ; rd ; ) {
			if((rd->last_collected_time.tv_sec + (10 * st->update_every)) < st->last_collected_time.tv_sec) { // remove it only it is not updated in 10 seconds
				debug(D_RRD_STATS, "Removing obsolete dimension '%s' (%s) of '%s' (%s).", rd->name, rd->id, st->name, st->id);

				if(!last) {
					st->dimensions = rd->next;
					rd->next = NULL;
					rrd_stats_dimension_free(rd);
					rd = st->dimensions;
					continue;
				}
				else {
					last->next = rd->next;
					rd->next = NULL;
					rrd_stats_dimension_free(rd);
					rd = last->next;
					continue;
				}
			}

			last = rd;
			rd = rd->next;
		}

		if(!st->dimensions) st->enabled = 0;
	}

	pthread_rwlock_unlock(&st->rwlock);

	if(pthread_setcancelstate(oldstate, NULL) != 0)
		error("Cannot set pthread cancel state to RESTORE (%d).", oldstate);

	return(st->usec_since_last_update);
}


// find the oldest entry in the data, skipping all empty slots
time_t rrd_stats_first_entry_t(RRD_STATS *st)
{
	if(!st->first_entry_t) return st->last_updated.tv_sec;
	
	return st->first_entry_t / 1000000;
}
