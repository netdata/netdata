#include <sys/time.h>
#include <pthread.h>

#include "storage_number.h"

#ifndef NETDATA_RRD_H
#define NETDATA_RRD_H 1

#define UPDATE_EVERY 1
#define UPDATE_EVERY_MAX 3600
extern int update_every;

#define HISTORY 3600
#define HISTORY_MAX (86400*10)
extern int save_history;

typedef long long total_number;

#define TOTAL_NUMBER_FORMAT "%lld"

#define RRD_STATS_NAME_MAX 1024

#define RRD_STATS_MAGIC     "NETDATA CACHE STATS FILE V009"
#define RRD_DIMENSION_MAGIC "NETDATA CACHE DIMENSION FILE V009"


// ----------------------------------------------------------------------------
// chart types

#define CHART_TYPE_LINE_NAME "line"
#define CHART_TYPE_AREA_NAME "area"
#define CHART_TYPE_STACKED_NAME "stacked"

#define CHART_TYPE_LINE	0
#define CHART_TYPE_AREA 1
#define CHART_TYPE_STACKED 2

int chart_type_id(const char *name);
const char *chart_type_name(int chart_type);


// ----------------------------------------------------------------------------
// memory mode

#define NETDATA_MEMORY_MODE_RAM_NAME "ram"
#define NETDATA_MEMORY_MODE_MAP_NAME "map"
#define NETDATA_MEMORY_MODE_SAVE_NAME "save"

#define NETDATA_MEMORY_MODE_RAM 0
#define NETDATA_MEMORY_MODE_MAP 1
#define NETDATA_MEMORY_MODE_SAVE 2

extern int memory_mode;

extern const char *memory_mode_name(int id);
extern int memory_mode_id(const char *name);


// ----------------------------------------------------------------------------
// algorithms types

#define RRD_DIMENSION_ABSOLUTE_NAME 			"absolute"
#define RRD_DIMENSION_INCREMENTAL_NAME 			"incremental"
#define RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL_NAME	"percentage-of-incremental-row"
#define RRD_DIMENSION_PCENT_OVER_ROW_TOTAL_NAME		"percentage-of-absolute-row"

#define RRD_DIMENSION_ABSOLUTE			0
#define RRD_DIMENSION_INCREMENTAL		1
#define RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL 	2
#define RRD_DIMENSION_PCENT_OVER_ROW_TOTAL 	3

extern int algorithm_id(const char *name);
extern const char *algorithm_name(int chart_type);

struct rrd_dimension {
	char magic[sizeof(RRD_DIMENSION_MAGIC) + 1];	// our magic
	char id[RRD_STATS_NAME_MAX + 1];				// the id of this dimension (for internal identification)
	char *name;										// the name of this dimension (as presented to user)
	char cache_file[FILENAME_MAX+1];
	
	unsigned long hash;								// a simple hash on the id, to speed up searching
													// we first compare hashes, and only if the hashes are equal we do string comparisons

	long entries;									// how many entries this dimension has
													// this should be the same to the entries of the data set

	long current_entry;								// the entry that is currently being updated
	int update_every;								// every how many seconds is this updated?

	int hidden;										// if set to non zero, this dimension will not be sent to the client
	int mapped;										// 1 if the file is mapped
	unsigned long memsize;							// the memory allocated for this dimension

	int algorithm;
	long multiplier;
	long divisor;

	struct timeval last_collected_time;				// when was this dimension last updated
													// this is only used to detect un-updated dimensions
													// which are removed after some time

	calculated_number calculated_value;
	calculated_number last_calculated_value;

	collected_number collected_value;				// the value collected at this round
	collected_number last_collected_value;			// the value that was collected at the last round

	struct rrd_dimension *next;						// linking of dimensions within the same data set

	storage_number values[];						// the array of values - THIS HAS TO BE THE LAST MEMBER
};
typedef struct rrd_dimension RRD_DIMENSION;

struct rrd_stats {
	char magic[sizeof(RRD_STATS_MAGIC) + 1];		// our magic

	char id[RRD_STATS_NAME_MAX + 1];				// id of the data set
	char *name;										// name of the data set
	char *cache_dir;								// the directory to store dimension maps
	char cache_file[FILENAME_MAX+1];

	char *type;										// the type of graph RRD_TYPE_* (a category, for determining graphing options)
	char *family;									// the family of this data set (for grouping them together)
	char *title;									// title shown to user
	char *units;									// units of measurement

	pthread_rwlock_t rwlock;
	unsigned long counter;							// the number of times we added values to this rrd
	unsigned long counter_done;						// the number of times we added values to this rrd

	int mapped;										// if set to 1, this is memory mapped
	unsigned long memsize;							// how much mem we have allocated for this (without dimensions)

	unsigned long hash_name;						// a simple hash on the name
	unsigned long hash;								// a simple hash on the id, to speed up searching
													// we first compare hashes, and only if the hashes are equal we do string comparisons

	long priority;

	long entries;									// total number of entries in the data set
	long current_entry;								// the entry that is currently being updated
													// it goes around in a round-robin fashion

	int update_every;								// every how many seconds is this updated?
	unsigned long long first_entry_t;				// the timestamp (in microseconds) of the oldest entry in the db
	struct timeval last_updated;					// when this data set was last updated (updated every time the rrd_stats_done() function)
	struct timeval last_collected_time;				// 
	unsigned long long usec_since_last_update;

	total_number collected_total;
	total_number last_collected_total;

	int chart_type;
	int debug;
	int enabled;
	int isdetail;									// if set, the data set should be considered as a detail of another
													// (the master data set should be the one that has the same family and is not detail)

	RRD_DIMENSION *dimensions;						// the actual data for every dimension

	struct rrd_stats *next;							// linking of rrd stats
};
typedef struct rrd_stats RRD_STATS;

extern RRD_STATS *root;
extern pthread_rwlock_t root_rwlock;

extern char *rrd_stats_strncpy_name(char *to, const char *from, int length);
extern void rrd_stats_set_name(RRD_STATS *st, const char *name);

extern char *rrd_stats_cache_dir(const char *id);

extern void rrd_stats_reset(RRD_STATS *st);

extern RRD_STATS *rrd_stats_create(const char *type, const char *id, const char *name, const char *family, const char *title, const char *units, long priority, int update_every, int chart_type);

extern RRD_DIMENSION *rrd_stats_dimension_add(RRD_STATS *st, const char *id, const char *name, long multiplier, long divisor, int algorithm);

extern void rrd_stats_dimension_set_name(RRD_STATS *st, RRD_DIMENSION *rd, const char *name);
extern void rrd_stats_dimension_free(RRD_DIMENSION *rd);

extern void rrd_stats_free_all(void);
extern void rrd_stats_save_all(void);

extern RRD_STATS *rrd_stats_find(const char *id);
extern RRD_STATS *rrd_stats_find_bytype(const char *type, const char *id);
extern RRD_STATS *rrd_stats_find_byname(const char *name);

extern RRD_DIMENSION *rrd_stats_dimension_find(RRD_STATS *st, const char *id);

extern int rrd_stats_dimension_hide(RRD_STATS *st, const char *id);

extern void rrd_stats_dimension_set_by_pointer(RRD_STATS *st, RRD_DIMENSION *rd, collected_number value);
extern int rrd_stats_dimension_set(RRD_STATS *st, const char *id, collected_number value);

extern void rrd_stats_next_usec(RRD_STATS *st, unsigned long long microseconds);
extern void rrd_stats_next(RRD_STATS *st);
extern void rrd_stats_next_plugins(RRD_STATS *st);

extern unsigned long long rrd_stats_done(RRD_STATS *st);

extern time_t rrd_stats_first_entry_t(RRD_STATS *st);

#endif /* NETDATA_RRD_H */
