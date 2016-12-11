#ifndef NETDATA_RRD_H
#define NETDATA_RRD_H 1

#define UPDATE_EVERY 1
#define UPDATE_EVERY_MAX 3600
extern int rrd_update_every;

#define RRD_DEFAULT_HISTORY_ENTRIES 3600
#define RRD_HISTORY_ENTRIES_MAX (86400*10)
extern int rrd_default_history_entries;

// time in seconds to delete unupdated dimensions
// set to zero to disable this feature
extern int rrd_delete_unupdated_dimensions;

#define RRD_ID_LENGTH_MAX 400

#define RRDSET_MAGIC        "NETDATA RRD SET FILE V018"
#define RRDDIMENSION_MAGIC  "NETDATA RRD DIMENSION FILE V018"

typedef long long total_number;
#define TOTAL_NUMBER_FORMAT "%lld"

// ----------------------------------------------------------------------------
// chart types

#define RRDSET_TYPE_LINE_NAME "line"
#define RRDSET_TYPE_AREA_NAME "area"
#define RRDSET_TYPE_STACKED_NAME "stacked"

#define RRDSET_TYPE_LINE    0
#define RRDSET_TYPE_AREA    1
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

#define RRDDIM_ABSOLUTE_NAME                "absolute"
#define RRDDIM_INCREMENTAL_NAME             "incremental"
#define RRDDIM_PCENT_OVER_DIFF_TOTAL_NAME   "percentage-of-incremental-row"
#define RRDDIM_PCENT_OVER_ROW_TOTAL_NAME    "percentage-of-absolute-row"

#define RRDDIM_ABSOLUTE                 0
#define RRDDIM_INCREMENTAL              1
#define RRDDIM_PCENT_OVER_DIFF_TOTAL    2
#define RRDDIM_PCENT_OVER_ROW_TOTAL     3

extern int rrddim_algorithm_id(const char *name);
extern const char *rrddim_algorithm_name(int chart_type);

// ----------------------------------------------------------------------------
// flags

#define RRDDIM_FLAG_HIDDEN 0x00000001 // this dimension will not be offered to callers
#define RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS 0x00000002 // do not offer RESET or OVERFLOW info to callers

// ----------------------------------------------------------------------------
// RRD CONTEXT

struct rrdfamily {
    avl avl;

    const char *family;
    uint32_t hash_family;

    size_t use_count;

    avl_tree_lock variables_root_index;
};
typedef struct rrdfamily RRDFAMILY;

// ----------------------------------------------------------------------------
// RRD DIMENSION

struct rrddim {
    // ------------------------------------------------------------------------
    // binary indexing structures

    avl avl;                                        // the binary index - this has to be first member!

    // ------------------------------------------------------------------------
    // the dimension definition

    char id[RRD_ID_LENGTH_MAX + 1];                 // the id of this dimension (for internal identification)

    const char *name;                               // the name of this dimension (as presented to user)
                                                    // this is a pointer to the config structure
                                                    // since the config always has a higher priority
                                                    // (the user overwrites the name of the charts)

    int algorithm;                                  // the algorithm that is applied to add new collected values
    long multiplier;                                // the multiplier of the collected values
    long divisor;                                   // the divider of the collected values

    int mapped;                                     // if set to non zero, this dimension is mapped to a file

    // ------------------------------------------------------------------------
    // members for temporary data we need for calculations

    uint32_t hash;                                  // a simple hash of the id, to speed up searching / indexing
                                                    // instead of strcmp() every item in the binary index
                                                    // we first compare the hashes

    // FIXME
    // we need the hash_name too!

    uint32_t flags;

    char cache_filename[FILENAME_MAX+1];            // the filename we load/save from/to this set

    unsigned long counter;                          // the number of times we added values to this rrdim

    int updated;                                    // set to 0 after each calculation, to 1 after each collected value
                                                    // we use this to detect that a dimension is not updated

    struct timeval last_collected_time;             // when was this dimension last updated
                                                    // this is actual date time we updated the last_collected_value
                                                    // THIS IS DIFFERENT FROM THE SAME MEMBER OF RRDSET

    calculated_number calculated_value;             // the current calculated value, after applying the algorithm - resets to zero after being used
    calculated_number last_calculated_value;        // the last calculated value processed

    calculated_number last_stored_value;            // the last value as stored in the database (after interpolation)

    collected_number collected_value;               // the current value, as collected - resets to 0 after being used
    collected_number last_collected_value;          // the last value that was collected, after being processed

    // the *_volume members are used to calculate the accuracy of the rounding done by the
    // storage number - they are printed to debug.log when debug is enabled for a set.
    calculated_number collected_volume;             // the sum of all collected values so far
    calculated_number stored_volume;                // the sum of all stored values so far

    struct rrddim *next;                            // linking of dimensions within the same data set
    struct rrdset *rrdset;

    // ------------------------------------------------------------------------
    // members for checking the data when loading from disk

    long entries;                                   // how many entries this dimension has in ram
                                                    // this is the same to the entries of the data set
                                                    // we set it here, to check the data when we load it from disk.

    int update_every;                               // every how many seconds is this updated

    unsigned long memsize;                          // the memory allocated for this dimension

    char magic[sizeof(RRDDIMENSION_MAGIC) + 1];     // a string to be saved, used to identify our data file

    struct rrddimvar *variables;

    // ------------------------------------------------------------------------
    // the values stored in this dimension, using our floating point numbers

    storage_number values[];                        // the array of values - THIS HAS TO BE THE LAST MEMBER
};
typedef struct rrddim RRDDIM;


// ----------------------------------------------------------------------------
// RRDSET

struct rrdset {
    // ------------------------------------------------------------------------
    // binary indexing structures

    avl avl;                                        // the index, with key the id - this has to be first!
    avl avlname;                                    // the index, with key the name

    // ------------------------------------------------------------------------
    // the set configuration

    char id[RRD_ID_LENGTH_MAX + 1];                 // id of the data set

    const char *name;                               // the name of this dimension (as presented to user)
                                                    // this is a pointer to the config structure
                                                    // since the config always has a higher priority
                                                    // (the user overwrites the name of the charts)

    char *type;                                     // the type of graph RRD_TYPE_* (a category, for determining graphing options)
    char *family;                                   // grouping sets under the same family
    char *title;                                    // title shown to user
    char *units;                                    // units of measurement

    char *context;                                  // the template of this data set
    uint32_t hash_context;

    int chart_type;

    int update_every;                               // every how many seconds is this updated?

    long entries;                                   // total number of entries in the data set

    long current_entry;                             // the entry that is currently being updated
                                                    // it goes around in a round-robin fashion

    int enabled;

    int gap_when_lost_iterations_above;             // after how many lost iterations a gap should be stored
                                                    // netdata will interpolate values for gaps lower than this

    long priority;

    int isdetail;                                   // if set, the data set should be considered as a detail of another
                                                    // (the master data set should be the one that has the same family and is not detail)

    // ------------------------------------------------------------------------
    // members for temporary data we need for calculations

    int mapped;                                     // if set to 1, this is memory mapped

    int debug;

    char *cache_dir;                                // the directory to store dimensions
    char cache_filename[FILENAME_MAX+1];            // the filename to store this set

    pthread_rwlock_t rwlock;

    unsigned long counter;                          // the number of times we added values to this rrd
    unsigned long counter_done;                     // the number of times we added values to this rrd

    uint32_t hash;                                  // a simple hash on the id, to speed up searching
                                                    // we first compare hashes, and only if the hashes are equal we do string comparisons

    uint32_t hash_name;                             // a simple hash on the name

    usec_t usec_since_last_update;                  // the time in microseconds since the last collection of data

    struct timeval last_updated;                    // when this data set was last updated (updated every time the rrd_stats_done() function)
    struct timeval last_collected_time;             // when did this data set last collected values

    total_number collected_total;                   // used internally to calculate percentages
    total_number last_collected_total;              // used internally to calculate percentages

    RRDFAMILY *rrdfamily;
    struct rrdhost *rrdhost;

    struct rrdset *next;                            // linking of rrdsets

    // ------------------------------------------------------------------------
    // local variables

    calculated_number green;
    calculated_number red;

    avl_tree_lock variables_root_index;
    RRDSETVAR *variables;
    RRDCALC *alarms;

    // ------------------------------------------------------------------------
    // members for checking the data when loading from disk

    unsigned long memsize;                          // how much mem we have allocated for this (without dimensions)

    char magic[sizeof(RRDSET_MAGIC) + 1];           // our magic

    // ------------------------------------------------------------------------
    // the dimensions

    avl_tree_lock dimensions_index;                 // the root of the dimensions index
    RRDDIM *dimensions;                             // the actual data for every dimension

};
typedef struct rrdset RRDSET;

// ----------------------------------------------------------------------------
// RRD HOST

struct rrdhost {
    avl avl;

    char *hostname;

    RRDSET *rrdset_root;
    pthread_rwlock_t rrdset_root_rwlock;

    avl_tree_lock rrdset_root_index;
    avl_tree_lock rrdset_root_index_name;

    avl_tree_lock rrdfamily_root_index;
    avl_tree_lock variables_root_index;

    // all RRDCALCs are primarily allocated and linked here
    // RRDCALCs may be linked to charts at any point
    // (charts may or may not exist when these are loaded)
    RRDCALC *alarms;
    ALARM_LOG health_log;

    RRDCALCTEMPLATE *templates;
};
typedef struct rrdhost RRDHOST;
extern RRDHOST localhost;
extern void rrdhost_init(char *hostname);

#ifdef NETDATA_INTERNAL_CHECKS
#define rrdhost_check_wrlock(host) rrdhost_check_wrlock_int(host, __FILE__, __FUNCTION__, __LINE__)
#define rrdhost_check_rdlock(host) rrdhost_check_rdlock_int(host, __FILE__, __FUNCTION__, __LINE__)
#else
#define rrdhost_check_rdlock(host) (void)0
#define rrdhost_check_wrlock(host) (void)0
#endif

extern void rrdhost_check_wrlock_int(RRDHOST *host, const char *file, const char *function, const unsigned long line);
extern void rrdhost_check_rdlock_int(RRDHOST *host, const char *file, const char *function, const unsigned long line);

extern void rrdhost_rwlock(RRDHOST *host);
extern void rrdhost_rdlock(RRDHOST *host);
extern void rrdhost_unlock(RRDHOST *host);

// ----------------------------------------------------------------------------
// RRD SET functions

extern char *rrdset_strncpyz_name(char *to, const char *from, size_t length);
extern void rrdset_set_name(RRDSET *st, const char *name);

extern char *rrdset_cache_dir(const char *id);

extern void rrdset_reset(RRDSET *st);

extern RRDSET *rrdset_create(const char *type
        , const char *id
        , const char *name
        , const char *family
        , const char *context
        , const char *title
        , const char *units
        , long priority
        , int update_every
        , int chart_type);

extern void rrdset_free_all(void);
extern void rrdset_save_all(void);

extern RRDSET *rrdset_find(const char *id);
extern RRDSET *rrdset_find_bytype(const char *type, const char *id);
extern RRDSET *rrdset_find_byname(const char *name);

extern void rrdset_next_usec_unfiltered(RRDSET *st, usec_t microseconds);
extern void rrdset_next_usec(RRDSET *st, usec_t microseconds);
#define rrdset_next(st) rrdset_next_usec(st, 0ULL)

extern usec_t rrdset_done(RRDSET *st);

// get the total duration in seconds of the round robin database
#define rrdset_duration(st) ((time_t)( (((st)->counter >= ((unsigned long)(st)->entries))?(unsigned long)(st)->entries:(st)->counter) * (st)->update_every ))

// get the timestamp of the last entry in the round robin database
#define rrdset_last_entry_t(st) ((time_t)(((st)->last_updated.tv_sec)))

// get the timestamp of first entry in the round robin database
#define rrdset_first_entry_t(st) ((time_t)(rrdset_last_entry_t(st) - rrdset_duration(st)))

// get the last slot updated in the round robin database
#define rrdset_last_slot(st) ((unsigned long)(((st)->current_entry == 0) ? (st)->entries - 1 : (st)->current_entry - 1))

// get the first / oldest slot updated in the round robin database
#define rrdset_first_slot(st) ((unsigned long)( (((st)->counter >= ((unsigned long)(st)->entries)) ? (unsigned long)( ((unsigned long)(st)->current_entry > 0) ? ((unsigned long)(st)->current_entry) : ((unsigned long)(st)->entries) ) - 1 : 0) ))

// get the slot of the round robin database, for the given timestamp (t)
// it always returns a valid slot, although may not be for the time requested if the time is outside the round robin database
#define rrdset_time2slot(st, t) ( \
        (  (time_t)(t) >= rrdset_last_entry_t(st))  ? ( rrdset_last_slot(st) ) : \
        ( ((time_t)(t) <= rrdset_first_entry_t(st)) ?   rrdset_first_slot(st) : \
        ( (rrdset_last_slot(st) >= (unsigned long)((rrdset_last_entry_t(st) - (time_t)(t)) / (unsigned long)((st)->update_every)) ) ? \
          (rrdset_last_slot(st) -  (unsigned long)((rrdset_last_entry_t(st) - (time_t)(t)) / (unsigned long)((st)->update_every)) ) : \
          (rrdset_last_slot(st) -  (unsigned long)((rrdset_last_entry_t(st) - (time_t)(t)) / (unsigned long)((st)->update_every)) + (unsigned long)(st)->entries ) \
        )))

// get the timestamp of a specific slot in the round robin database
#define rrdset_slot2time(st, slot) ( rrdset_last_entry_t(st) - \
        ((unsigned long)(st)->update_every * ( \
                ( (unsigned long)(slot) > rrdset_last_slot(st)) ? \
                ( (rrdset_last_slot(st) - (unsigned long)(slot) + (unsigned long)(st)->entries) ) : \
                ( (rrdset_last_slot(st) - (unsigned long)(slot)) )) \
        ))

// ----------------------------------------------------------------------------
// RRD DIMENSION functions

extern RRDDIM *rrddim_add(RRDSET *st, const char *id, const char *name, long multiplier, long divisor, int algorithm);

extern void rrddim_set_name(RRDSET *st, RRDDIM *rd, const char *name);
extern void rrddim_free(RRDSET *st, RRDDIM *rd);

extern RRDDIM *rrddim_find(RRDSET *st, const char *id);

extern int rrddim_hide(RRDSET *st, const char *id);
extern int rrddim_unhide(RRDSET *st, const char *id);

extern collected_number rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, collected_number value);
extern collected_number rrddim_set(RRDSET *st, const char *id, collected_number value);

#endif /* NETDATA_RRD_H */
