#ifndef NETDATA_RRD_H
#define NETDATA_RRD_H 1

/**
 * @file rrd.h
 * @brief The round robin database.
 *
 * The round robin database is a in memory database. It stores `RRDSET` (charts)
 * which contain `RRDDIM` (dimensions). Dimensions contain time related values.
 */

#define UPDATE_EVERY 1        ///< Default update interval in seconds.
#define UPDATE_EVERY_MAX 3600 ///< Maximum update interval in seconds.
extern int rrd_update_every;  ///< Current update interval in seconds.

#define RRD_DEFAULT_HISTORY_ENTRIES 3600     ///< Default history size
#define RRD_HISTORY_ENTRIES_MAX (86400 * 10) ///< Maximum history size
extern int rrd_default_history_entries;      ///< Current history size

/// Time in seconds to delete unupdated dimensions.
///
/// Set to zero to disable this feature.
extern int rrd_delete_unupdated_dimensions;

#define RRD_ID_LENGTH_MAX 400 ///< Maximum length of RRDDIM->id

#define RRDSET_MAGIC "NETDATA RRD SET FILE V018"             ///< Magic pattern of round robin database set files @see man 5 magic
#define RRDDIMENSION_MAGIC "NETDATA RRD DIMENSION FILE V018" ///< Magic pattern of round robin database dimension files @see man 5 magic

typedef long long total_number;    ///< ktsaou: Your help needed.
#define TOTAL_NUMBER_FORMAT "%lld" ///< Format string escape sequence of `total_number` @see man 3 printf

// ----------------------------------------------------------------------------
// chart types

#define RRDSET_TYPE_LINE_NAME "line"       ///< String representation of #RRDSET_TYPE_LINE.
#define RRDSET_TYPE_AREA_NAME "area"       ///< String representation of #RRDSET_TYPE_AREA
#define RRDSET_TYPE_STACKED_NAME "stacked" ///< String representation of #RRDSET_TYPE_STACKED

#define RRDSET_TYPE_LINE 0    ///< Charts should display each dimension as a line.
#define RRDSET_TYPE_AREA 1    ///< Charts should display each dimension as an independen area
#define RRDSET_TYPE_STACKED 2 ///< Charts should stack dimensions.

/**
 * Get integer representation for string representation of RRDSET_TYPE_*.
 *
 * @param name sting representation
 * @return integer representation
 */
int rrdset_type_id(const char *name);
/**
 * Get string representation for integer representation of RRDSET_TYPE_*.
 *
 * @param chart_type integer represenatation
 * @return string represenatation
 */
const char *rrdset_type_name(int chart_type);

// ----------------------------------------------------------------------------
// memory mode

#define RRD_MEMORY_MODE_RAM_NAME "ram"   ///< String representation of #RRD_MEMORY_MODE_RAM
#define RRD_MEMORY_MODE_MAP_NAME "map"   ///< String representation of #RRD_MEMORY_MODE_MAP
#define RRD_MEMORY_MODE_SAVE_NAME "save" ///< String representation of #RRD_MEMORY_MODE_SAVE

#define RRD_MEMORY_MODE_RAM 0  ///< Store round robin database in memory
#define RRD_MEMORY_MODE_MAP 1  ///< ktsaou your help needed
#define RRD_MEMORY_MODE_SAVE 2 ///< ktsaou your help needed

extern int rrd_memory_mode; ///< Curren memory mode. (RRD_MEMORY_MODE_*)

/**
 * Get string representation for integer representation of RRD_MEMORY_MODE_*
 *
 * @param id integer represenation
 * @return string representation
 */
extern const char *rrd_memory_mode_name(int id);
/**
 * Get integer representation for string representation of RRD_MEMORY_MODE_*
 *
 * @param name represenation
 * @return integer representation
 */
extern int rrd_memory_mode_id(const char *name);

// ----------------------------------------------------------------------------
// algorithms types

#define RRDDIM_ABSOLUTE_NAME "absolute"                                   ///< String representation of #RRDDIM_ABSOLUTE
#define RRDDIM_INCREMENTAL_NAME "incremental"                             ///< String representation of #RRDDIM_INCREMENTAL
#define RRDDIM_PCENT_OVER_DIFF_TOTAL_NAME "percentage-of-incremental-row" ///< String representation of #RRDDIM_PCENT_OVER_DIFF_TOTAL
#define RRDDIM_PCENT_OVER_ROW_TOTAL_NAME "percentage-of-absolute-row"     ///< String representation of #RRDDIM_PCENT_OVER_ROW_TOTAL

#define RRDDIM_ABSOLUTE 0              ///< Store value.
#define RRDDIM_INCREMENTAL 1           ///< Store the difference form the last value.
#define RRDDIM_PCENT_OVER_DIFF_TOTAL 2 ///< Store percentage of the difference form the last value compared to the sum of the difference from the last value of all dimension.
#define RRDDIM_PCENT_OVER_ROW_TOTAL 3  ///< Store percentage of value compared to the total of all dimensions. 

/**
 * Get integer representation for string representation of RRDDIM_*
 *
 * @param name string represenation
 * @return integer representation
 */
extern int rrddim_algorithm_id(const char *name);
/**
 * Get string representation for integer representation of RRDDIM_*
 *
 * @param chart_type integer represenation
 * @return string representation
 */
extern const char *rrddim_algorithm_name(int chart_type);

// ----------------------------------------------------------------------------
// flags

#define RRDDIM_FLAG_HIDDEN 0x00000001                          ///< this dimension will not be offered to callers
#define RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS 0x00000002 ///< do not offer RESET or OVERFLOW info to callers

// ----------------------------------------------------------------------------
// RRD CONTEXT

/** Family to group rrdset. */
struct rrdfamily
{
    avl avl; ///< The tree

    const char *family;   ///< Name of family.
    uint32_t hash_family; ///< hash of `family`

    size_t use_count; ///< Statistic of access

    avl_tree_lock variables_root_index; ///< Variables index
};
typedef struct rrdfamily RRDFAMILY; ///< Family to sub-group rrdset.

// ----------------------------------------------------------------------------
// RRD DIMENSION

/** Round robin database dimension. */
struct rrddim
{
    // ------------------------------------------------------------------------
    // binary indexing structures

    avl avl; ///< the binary index - this has to be first member!

    // ------------------------------------------------------------------------
    // the dimension definition

    char id[RRD_ID_LENGTH_MAX + 1]; ///< the id of this dimension (for internal identification)

    const char *name; ///<  the name of this dimension (as presented to user)
                      ///< this is a pointer to the config structure
                      ///< since the config always has a higher priority
                      ///< (the user overwrites the name of the charts)

    int algorithm;   ///< the algorithm that is applied to add new collected values
    long multiplier; ///< the multiplier of the collected values
    long divisor;    ///< the divider of the collected values

    int mapped; ///< if set to non zero, this dimension is mapped to a file

    // ------------------------------------------------------------------------
    // members for temporary data we need for calculations

    uint32_t hash; ///< a simple hash of the id, to speed up searching / indexing
                   ///< instead of strcmp() every item in the binary index
                   ///< we first compare the hashes

    // FIXME
    // we need the hash_name too!
    // needed at rrdr_disable_not_selected_dimensions()

    uint32_t flags; ///< RRDIM_FLAG_*

    char cache_filename[FILENAME_MAX + 1]; ///< the filename we load/save from/to this set

    unsigned long counter; ///< the number of times we added values to this rrdim

    int updated; ///< set to 0 after each calculation, to 1 after each collected value
                 ///< we use this to detect that a dimension is not updated

    struct timeval last_collected_time; ///< when was this dimension last updated
                                        ///< this is actual date time we updated the last_collected_value
                                        ///< THIS IS DIFFERENT FROM THE SAME MEMBER OF RRDSET

    calculated_number calculated_value;      ///< the current calculated value, after applying the algorithm - resets to zero after being used
    calculated_number last_calculated_value; ///< the last calculated value processed

    calculated_number last_stored_value; ///< the last value as stored in the database (after interpolation)

    collected_number collected_value;      ///< the current value, as collected - resets to 0 after being used
    collected_number last_collected_value; ///< the last value that was collected, after being processed

    // the *_volume members are used to calculate the accuracy of the rounding done by the
    // storage number - they are printed to debug.log when debug is enabled for a set.
    calculated_number collected_volume; ///< the sum of all collected values so far
    calculated_number stored_volume;    ///< the sum of all stored values so far

    struct rrddim *next;   ///< linking of dimensions within the same data set
    struct rrdset *rrdset; ///< round robin database set

    // ------------------------------------------------------------------------
    // members for checking the data when loading from disk

    long entries; ///< how many entries this dimension has in ram
                  ///< this is the same to the entries of the data set
                  ///< we set it here, to check the data when we load it from disk.

    int update_every; ///< every how many seconds is this updated

    unsigned long memsize; ///< the memory allocated for this dimension

    char magic[sizeof(RRDDIMENSION_MAGIC) + 1]; ///< a string to be saved, used to identify our data file

    struct rrddimvar *variables; ///< health variables

    // ------------------------------------------------------------------------
    // the values stored in this dimension, using our floating point numbers

    storage_number values[]; ///< the array of values - THIS HAS TO BE THE LAST MEMBER
};
typedef struct rrddim RRDDIM; ///< Round robin database dimension.

// ----------------------------------------------------------------------------
// RRDSET

/** Round robin database set. */
struct rrdset
{
    // ------------------------------------------------------------------------
    // binary indexing structures

    avl avl;     ///< the index, with key the id - this has to be first!
    avl avlname; ///< the index, with key the name

    // ------------------------------------------------------------------------
    // the set configuration

    char id[RRD_ID_LENGTH_MAX + 1]; ///< id of the data set

    const char *name; ///< the name of this dimension (as presented to user)
                      ///< this is a pointer to the config structure
                      ///< since the config always has a higher priority
                      ///< (the user overwrites the name of the charts)

    char *type;   ///< the type of graph RRD_TYPE_* (a category, for determining graphing options)
    char *family; ///< grouping sets under the same family
    char *title;  ///< title shown to user
    char *units;  ///< units of measurement

    char *context;         ///< the template of this data set
    uint32_t hash_context; ///< hash of `context`

    int chart_type; ///< RRDSET_TYPE_LINE|RRDSET_TYPE_AREA|RRDSET_TYPE_STACKED

    int update_every; ///< every how many seconds is this updated?

    long entries; ///< total number of entries in the data set

    long current_entry; ///< the entry that is currently being updated
                        ///< it goes around in a round-robin fashion

    int enabled; ///< boolean

    int gap_when_lost_iterations_above; ///< after how many lost iterations a gap should be stored
                                        ///< netdata will interpolate values for gaps lower than this

    long priority; ///< used for sorting

    int isdetail; ///< if set, the data set should be considered as a detail of another
                  ///< (the master data set should be the one that has the same family and is not detail)

    // ------------------------------------------------------------------------
    // members for temporary data we need for calculations

    int mapped; ///< if set to 1, this is memory mapped

    int debug; ///< boolean to enable debugging output for this set

    char *cache_dir;                       ///< the directory to store dimensions
    char cache_filename[FILENAME_MAX + 1]; ///< the filename to store this set

    pthread_rwlock_t rwlock; ///< Lock to synchronize access this set

    unsigned long counter;      ///< the number of times we added values to this rrd
    unsigned long counter_done; ///< the number of times we added values to this rrd

    uint32_t hash; ///< a simple hash on the id, to speed up searching
                   ///< we first compare hashes, and only if the hashes are equal we do string comparisons

    uint32_t hash_name; ///< a simple hash on the name

    usec_t usec_since_last_update; ///< the time in microseconds since the last collection of data

    struct timeval last_updated;        ///< when this data set was last updated (updated every time the rrd_stats_done() function)
    struct timeval last_collected_time; ///< when did this data set last collected values

    total_number collected_total;      ///< used internally to calculate percentages
    total_number last_collected_total; ///< used internally to calculate percentages

    RRDFAMILY *rrdfamily;    ///< family this set is in
    struct rrdhost *rrdhost; ///< host this thread is mapped to

    struct rrdset *next; ///< linking of rrdsets

    // ------------------------------------------------------------------------
    // local variables

    calculated_number green; ///< the green threshold of this
    calculated_number red;   ///< the red threshold of this

    avl_tree_lock variables_root_index; ///< health variable index
    RRDSETVAR *variables;               ///< health variables
    RRDCALC *alarms;                    ///< alarms

    // ------------------------------------------------------------------------
    // members for checking the data when loading from disk

    unsigned long memsize; ///< how much mem we have allocated for this (without dimensions)

    char magic[sizeof(RRDSET_MAGIC) + 1]; ///< our magic

    // ------------------------------------------------------------------------
    // the dimensions

    avl_tree_lock dimensions_index; ///< the root of the dimensions index
    RRDDIM *dimensions;             ///< the actual data for every dimension
};
typedef struct rrdset RRDSET; ///< Round robin database set.

// ----------------------------------------------------------------------------
// RRD HOST

/** Round robin database host. */
struct rrdhost
{
    avl avl; ///< The AVL tree

    char *hostname; ///< hostname

    RRDSET *rrdset_root;                 ///< The root RRDSET
    pthread_rwlock_t rrdset_root_rwlock; ///< lock for `rrdset_root`

    avl_tree_lock rrdset_root_index;      ///< index of rrdsets
    avl_tree_lock rrdset_root_index_name; ///< index of rrdset names

    avl_tree_lock rrdfamily_root_index; ///< index of rrdfamalys
    avl_tree_lock variables_root_index; ///< index of variables

    /// all RRDCALCs are primarily allocated and linked here
    /// RRDCALCs may be linked to charts at any point
    /// (charts may or may not exist when these are loaded)
    RRDCALC *alarms;      ///< alarms
    ALARM_LOG health_log; ///< log for alarms

    RRDCALCTEMPLATE *templates; ///< claculation templates
};
typedef struct rrdhost RRDHOST; ///< Round robin database host.
extern RRDHOST localhost; ///< Local round robin database host.
/**
 * Initialize RRDHOST `localhost` with hostname.
 *
 * @param hostname to set.
 */
extern void rrdhost_init(char *hostname);

#ifdef NETDATA_INTERNAL_CHECKS
#define rrdhost_check_wrlock(host) rrdhost_check_wrlock_int(host, __FILE__, __FUNCTION__, __LINE__)
#define rrdhost_check_rdlock(host) rrdhost_check_rdlock_int(host, __FILE__, __FUNCTION__, __LINE__)
#else
/**
 * Check if host is locked for reading.
 *
 * If not print fatal message and quit the program.
 * This method is disabled at production.
 *
 * @param host RRDHOST to check
 */
#define rrdhost_check_rdlock(host) (void)0
/**
 * Check if host is locked for writing.
 *
 * If not print fatal message and quit the program.
 * This method is disabled at production.
 *
 * @param host RRDHOST to check
 */
#define rrdhost_check_wrlock(host) (void)0
#endif

/**
 * Check if host is locked for writing.
 *
 * Use rrdhost_check_wrlock() instead.
 *
 * @param host to check
 * @param file contains this call
 * @param function contains this call
 * @param line contains this call
 */
void rrdhost_check_wrlock_int(RRDHOST *host, const char *file, const char *function, const unsigned long line);
/**
 * Check if host is locked for writing.
 *
 * Use rrdhost_check_rdlock() instead.
 *
 * @param host to check
 * @param file contains this call
 * @param function contains this call
 * @param line contains this call
 */
void rrdhost_check_rdlock_int(RRDHOST *host, const char *file, const char *function, const unsigned long line);

/**
 * Lock host for reading.
 *
 * @param host to lock.
 */
extern void rrdhost_rwlock(RRDHOST *host);
/**
 * Lock host for writing.
 *
 * @param host to lock.
 */
extern void rrdhost_rdlock(RRDHOST *host);
/**
 * Unlock host.
 *
 * @param host to lock.
 */
extern void rrdhost_unlock(RRDHOST *host);

// ----------------------------------------------------------------------------
// RRD SET functions

/**
 * Copy at most length characters from `from` to `to`
 *
 * Not alphanumerical characters and '.' get replaced with '_'.
 * Destination always is terminated with '\0'
 *
 * @param to Destination.
 * @param from Source.
 * @param length Maximum characters to copy.
 * @return `to`
 */
extern char *rrdset_strncpyz_name(char *to, const char *from, size_t length);

/**
 * Set name of RRDSET.
 *
 * @param st RDDSET to (re)name.
 * @param name New name.
 */
extern void rrdset_set_name(RRDSET *st, const char *name);

/**
 * Get cache directoy of RRDSET.
 *
 * \todo ktsaou: Your help needed. Why this should be public as we provide this information at RRDSET->cache_dir.
 *
 * @param id of RRDSET. 
 * @return path to cache directory.
 */
extern char *rrdset_cache_dir(const char *id);

/**
 * Reset RRDSET.
 *
 * @param st RRDSET to reset.
 */
extern void rrdset_reset(RRDSET *st);

/**
 * Create new RRDSET.
 *
 * RDDSET needs a unique identifier `type`.`id`.
 * Name can be set optional. It is presented to the user instead of id.
 * `title` is the text displayed over the chart,
 * `units` the label of the vertical axis.
 * `type` groups RRDSET's, `family` controls the sub-group.
 * 
 * The `context` is giving the template of the chart.
 * For example, if multiple charts present the same information for a different family, they should have the same
 * context this is used for looking up rendering information for the chart (colors, sizes, informational texts) 
 * and also apply alarms to it.
 *
 * Use priority to sort the charts on the web page.
 * Lower numbers make the charts appear before the ones with higher numbers.
 * If NULL, 1000 will be used.
 *
 * Use `update_every` overwrite the update frequency of a single chart.
 * If NULL, the user configured or default value will be used.
 *
 * @param type controls the menu the chart will appear in
 * @param id together with type uniquely identifies the chart
 * @param name is the name that will be presented to the user instead of id
 * @param family controls the submenu the chart will apear in
 * @param context is giving the template of the chart.
 * @param title The text displayed above the chart.
 * @param units The label of the vertical axis of the chart.
 * @param priority is the relative priority of the charts as rendered on the web page.
 * @param update_every if set, overwrites the update frequency set by the server.
 * @param chart_type RRDSET_TYPE_*
 * @return the new RRDSET
 */
extern RRDSET *rrdset_create(const char *type
        , const char *id, const char *name, const char *family
        , const char *context, const char *title
        , const char *units
        , long priority
        , int update_every
        , int chart_type);

/**
 * Free all RRDSET of the database.
 */
extern void rrdset_free_all(void);
/**
 * Write all RRDSET of the database to disk.
 */
extern void rrdset_save_all(void);

/**
 * Get RRDSET by id.
 *
 * @param id RRDSET->id
 * @return RRDSET or NULL if not found
 */
extern RRDSET *rrdset_find(const char *id);
/**
 * Get RRDSET by type and id.
 *
 * @param type RRDSET->type
 * @param id RRDSET->id
 * @return RRDSET or NULL if not found
 */
extern RRDSET *rrdset_find_bytype(const char *type, const char *id);
/**
 * Get RRDSET by name.
 *
 * @param name RRDSET->name
 * @return RRDSET
 */
extern RRDSET *rrdset_find_byname(const char *name);

/**
 * ktsaou: Your help needed.
 *
 * @param st RRDSET.
 * @param microseconds ktsaou: Your help needed.
 */
extern void rrdset_next_usec_unfiltered(RRDSET *st, usec_t microseconds);
/**
 * ktsaou: Your help needed.
 *
 * @param st RRDSET.
 * @param microseconds ktsaou: Your help needed.
 */
extern void rrdset_next_usec(RRDSET *st, usec_t microseconds);
/**
 * ktsaou: Your help needed.
 *
 * @param st RRDSET.
 */
#define rrdset_next(st) rrdset_next_usec(st, 0ULL)

/**
 * Finish one data collection tick.
 *
 * @param st RRDSET
 * @return ktsaou: Your help needed.
 */
extern usec_t rrdset_done(RRDSET *st);

/**
 * Get the total duration in seconds of the round robin database.
 *
 * @param st RRDSET
 * @return duration of round robin database in seconds
 */
#define rrdset_duration(st) ((time_t)((((st)->counter >= ((unsigned long)(st)->entries)) ? (unsigned long)(st)->entries : (st)->counter) * (st)->update_every))

/**
 * Get the timestamp of the last entry in the round robin database.
 *
 * @return st RRDSET
 * @return time_t
 */
#define rrdset_last_entry_t(st) ((time_t)(((st)->last_updated.tv_sec)))

/**
 * Get the timestamp of first entry in the round robin database.
 *
 * @param st RRDSET
 * @return time_t
 */
#define rrdset_first_entry_t(st) ((time_t)(rrdset_last_entry_t(st) - rrdset_duration(st)))

/**
 * Get the last slot updated in the round robin database.
 * 
 * @param st RRDSET
 * @return the index of the last updated slot.
 */
#define rrdset_last_slot(st) ((unsigned long)(((st)->current_entry == 0) ? (st)->entries - 1 : (st)->current_entry - 1))

/**
 * Get the first / oldest slot updated in the round robin database.
 *
 * @param st RRDSET
 * @return the index of the oldest updated slot
 */
#define rrdset_first_slot(st) ((unsigned long)((((st)->counter >= ((unsigned long)(st)->entries)) ? (unsigned long)(((unsigned long)(st)->current_entry > 0) ? ((unsigned long)(st)->current_entry) : ((unsigned long)(st)->entries)) - 1 : 0)))

/**
 * Get the slot of the round robin database, for the given timestamp `t`.
 * 
 * It always returns a valid slot, although may not be for the time requested if the time is outside the round robin database.
 *
 * @param st RRDSET
 * @param t timestamp
 * @return the index for `timestamp`
 */
#define rrdset_time2slot(st, t) ( \
    ((time_t)(t) >= rrdset_last_entry_t(st)) ? (rrdset_last_slot(st)) : (((time_t)(t) <= rrdset_first_entry_t(st)) ? rrdset_first_slot(st) : ((rrdset_last_slot(st) >= (unsigned long)((rrdset_last_entry_t(st) - (time_t)(t)) / (unsigned long)((st)->update_every))) ? (rrdset_last_slot(st) - (unsigned long)((rrdset_last_entry_t(st) - (time_t)(t)) / (unsigned long)((st)->update_every))) : (rrdset_last_slot(st) - (unsigned long)((rrdset_last_entry_t(st) - (time_t)(t)) / (unsigned long)((st)->update_every)) + (unsigned long)(st)->entries))))

/**
 * Get the timestamp of a specific slot in the round robin database.
 *
 * @param st RRDSET
 * @param slot to return timestamp for
 * @return timestamp
 */
#define rrdset_slot2time(st, slot) (rrdset_last_entry_t(st) - \
                                    ((unsigned long)(st)->update_every * (((unsigned long)(slot) > rrdset_last_slot(st)) ? ((rrdset_last_slot(st) - (unsigned long)(slot) + (unsigned long)(st)->entries)) : ((rrdset_last_slot(st) - (unsigned long)(slot))))))

// ----------------------------------------------------------------------------
// RRD DIMENSION functions

/**
 * Add a new dimension to a RRDSET.
 *
 * @param st RRDSET.
 * @param id of this dimension.
 * @param name to expose to the user. If NULL id gets exposed.
 * @param multiplier to multiply the collected value before storing
 * @param divisor to divide the collected value before storing
 * @param algorithm RRDDIM_*
 * @return new RRDDIM
 */
extern RRDDIM *rrddim_add(RRDSET *st, const char *id, const char *name, long multiplier, long divisor, int algorithm);

/**
 * Set name of RRDDIM of RRDSET.
 *
 * @param st RDDSET.
 * @param rd RRDDIM to (re)name.
 * @param name New name.
 */
extern void rrddim_set_name(RRDSET *st, RRDDIM *rd, const char *name);
/**
 * Free RRDDIM of RRDSET.
 *
 * \todo ktsaou: Your help needed. Is it save to extern this or should this only be called from rrdset_free_all()?
 *
 * @param st RDDSET.
 * @param rd RRDDIM to free.
 */
extern void rrddim_free(RRDSET *st, RRDDIM *rd);

/**
 * Get RRDDIM of RRDSET by id.
 *
 * @param st RRDSET.
 * @param id of RRDDIM
 * @return RRDDIM or NULL
 */
extern RRDDIM *rrddim_find(RRDSET *st, const char *id);

/**
 * Hide RRDDIM of RRDSET with `id` on requests.
 *
 * @param st RRDSET.
 * @param id of RRDDIM
 * @return 0 on success, 1 on error.
 */
extern int rrddim_hide(RRDSET *st, const char *id);
/**
 * Unhide RRDDIM of RRDSET with `id` on requests.
 *
 * @param st RRDSET.
 * @param id of RRDDIM
 * @return 0 on success, 1 on error.
 */
extern int rrddim_unhide(RRDSET *st, const char *id);

/**
 * Add a value to RRDDIM.
 *
 * @param st RRDSET of RRDDIM.
 * @param rd RRDDIM.
 * @param value to add.
 * @return ktsaou: Your help needed. Last interpolated value?
 */
extern collected_number rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, collected_number value);
/**
 * Add a value to RRDDIM.
 *
 * @param st RRDSET of RRDDIM.
 * @param id of RRDDIM
 * @param value to add.
 * @return 0 if RRDDIM not found. \todo see rrddim_set_by_pointer()
 */
extern collected_number rrddim_set(RRDSET *st, const char *id, collected_number value);

#endif /* NETDATA_RRD_H */
