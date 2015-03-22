#include <sys/time.h>
#include <pthread.h>

#include "avl.h"
#include "storage_number.h"

#ifndef NETDATA_RRD_H
#define NETDATA_RRD_H 1

#define UPDATE_EVERY 1
#define UPDATE_EVERY_MAX 3600
extern int rrd_update_every;

#define RRD_DEFAULT_HISTORY_ENTRIES 3600
#define RRD_HISTORY_ENTRIES_MAX (86400*10)
extern int rrd_default_history_entries;

#define RRD_DEFAULT_GAP_INTERPOLATIONS 10

typedef long long total_number;
#define TOTAL_NUMBER_FORMAT "%lld"

#define RRD_ID_LENGTH_MAX 1024

#define RRDSET_MAGIC		"NETDATA CACHE STATS FILE V011"
#define RRDDIMENSION_MAGIC	"NETDATA CACHE DIMENSION FILE V011"

// ----------------------------------------------------------------------------
// chart types

#define RRDSET_TYPE_LINE_NAME "line"
#define RRDSET_TYPE_AREA_NAME "area"
#define RRDSET_TYPE_STACKED_NAME "stacked"

#define RRDSET_TYPE_LINE	0
#define RRDSET_TYPE_AREA 	1
#define RRDSET_TYPE_STACKED 2

int rrdset_type_id(const char *name);
const char *rrdset_type_name(int chart_type);


// ----------------------------------------------------------------------------
// memory mode

#define RRD_MEMORY_MODE_RAM_NAME "ram"
#define RRD_MEMORY_MODE_MAP_NAME "map"
#define RRD_MEMORY_MODE_SAVE_NAME "save"

#define RRD_MEMORY_MODE_RAM 0
#define RRD_MEMORY_MODE_MAP 1
#define RRD_MEMORY_MODE_SAVE 2

extern int rrd_memory_mode;

extern const char *rrd_memory_mode_name(int id);
extern int rrd_memory_mode_id(const char *name);


// ----------------------------------------------------------------------------
// algorithms types

#define RRDDIM_ABSOLUTE_NAME 				"absolute"
#define RRDDIM_INCREMENTAL_NAME 			"incremental"
#define RRDDIM_PCENT_OVER_DIFF_TOTAL_NAME	"percentage-of-incremental-row"
#define RRDDIM_PCENT_OVER_ROW_TOTAL_NAME	"percentage-of-absolute-row"

#define RRDDIM_ABSOLUTE					0
#define RRDDIM_INCREMENTAL				1
#define RRDDIM_PCENT_OVER_DIFF_TOTAL	2
#define RRDDIM_PCENT_OVER_ROW_TOTAL		3

extern int rrddim_algorithm_id(const char *name);
extern const char *rrddim_algorithm_name(int chart_type);


// ----------------------------------------------------------------------------
// RRD DIMENSION

struct rrddim {
	avl avl;

	char magic[sizeof(RRDDIMENSION_MAGIC) + 1];		// our magic
	char id[RRD_ID_LENGTH_MAX + 1];						// the id of this dimension (for internal identification)
	const char *name;								// the name of this dimension (as presented to user)
	char cache_filename[FILENAME_MAX+1];
	
	unsigned long hash;								// a simple hash on the id, to speed up searching
													// we first compare hashes, and only if the hashes are equal we do string comparisons

	long entries;									// how many entries this dimension has
													// this should be the same to the entries of the data set

	int update_every;								// every how many seconds is this updated?
	int updated;									// set to 0 after each calculation, to 1 after each collected value

	int hidden;										// if set to non zero, this dimension will not be sent to the client
	int mapped;										// 1 if the file is mapped
	unsigned long memsize;							// the memory allocated for this dimension

	int algorithm;
	long multiplier;
	long divisor;

	struct timeval last_collected_time;				// when was this dimension last updated
													// this is actual date time we updated the last_collected_value
													// THIS IS DIFFERENT FROM THE SAME MEMBER OF RRD_STATS

	calculated_number calculated_value;
	calculated_number last_calculated_value;

	collected_number collected_value;				// the value collected at this round
	collected_number last_collected_value;			// the value that was collected at the last round

	calculated_number collected_volume;
	calculated_number stored_volume;

	struct rrddim *next;						// linking of dimensions within the same data set

	storage_number values[];						// the array of values - THIS HAS TO BE THE LAST MEMBER
};
typedef struct rrddim RRDDIM;


// ----------------------------------------------------------------------------
// RRDSET

struct rrdset {
	avl avl;

	char magic[sizeof(RRDSET_MAGIC) + 1];			// our magic

	char id[RRD_ID_LENGTH_MAX + 1];						// id of the data set
	const char *name;								// name of the data set
	char *cache_dir;								// the directory to store dimension maps
	char cache_filename[FILENAME_MAX+1];

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

	int gap_when_lost_iterations_above;				// after how many lost iterations a gap should be stored
													// netdata will interpolate values for gaps lower than this

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

	RRDDIM *dimensions;								// the actual data for every dimension

	avl_tree dimensions_index;

	struct rrdset *next;							// linking of rrdsets
};
typedef struct rrdset RRDSET;

extern RRDSET *rrdset_root;
extern pthread_rwlock_t rrdset_root_rwlock;

// ----------------------------------------------------------------------------
// RRD SET functions

extern char *rrdset_strncpy_name(char *to, const char *from, int length);
extern void rrdset_set_name(RRDSET *st, const char *name);

extern char *rrdset_cache_dir(const char *id);

extern void rrdset_reset(RRDSET *st);

extern RRDSET *rrdset_create(const char *type, const char *id, const char *name, const char *family, const char *title, const char *units, long priority, int update_every, int chart_type);

extern void rrdset_free_all(void);
extern void rrdset_save_all(void);

extern RRDSET *rrdset_find(const char *id);
extern RRDSET *rrdset_find_bytype(const char *type, const char *id);
extern RRDSET *rrdset_find_byname(const char *name);

extern void rrdset_next_usec(RRDSET *st, unsigned long long microseconds);
extern void rrdset_next(RRDSET *st);
extern void rrdset_next_plugins(RRDSET *st);

extern unsigned long long rrdset_done(RRDSET *st);

extern time_t rrdset_first_entry_t(RRDSET *st);


// ----------------------------------------------------------------------------
// RRD DIMENSION functions

extern RRDDIM *rrddim_add(RRDSET *st, const char *id, const char *name, long multiplier, long divisor, int algorithm);

extern void rrddim_set_name(RRDSET *st, RRDDIM *rd, const char *name);
extern void rrddim_free(RRDSET *st, RRDDIM *rd);

extern RRDDIM *rrddim_find(RRDSET *st, const char *id);

extern int rrddim_hide(RRDSET *st, const char *id);

extern void rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, collected_number value);
extern int rrddim_set(RRDSET *st, const char *id, collected_number value);

#endif /* NETDATA_RRD_H */
