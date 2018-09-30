// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRD_H
#define NETDATA_RRD_H 1

#define UPDATE_EVERY 1
#define UPDATE_EVERY_MAX 3600

#define RRD_DEFAULT_HISTORY_ENTRIES 3600
#define RRD_HISTORY_ENTRIES_MAX (86400*365)

extern int default_rrd_update_every;
extern int default_rrd_history_entries;
extern int gap_when_lost_iterations_above;

#define RRD_ID_LENGTH_MAX 200

#define RRDSET_MAGIC        "NETDATA RRD SET FILE V019"
#define RRDDIMENSION_MAGIC  "NETDATA RRD DIMENSION FILE V019"

typedef long long total_number;
#define TOTAL_NUMBER_FORMAT "%lld"

typedef struct rrdhost RRDHOST;

// ----------------------------------------------------------------------------
// chart types

typedef enum rrdset_type {
    RRDSET_TYPE_LINE    = 0,
    RRDSET_TYPE_AREA    = 1,
    RRDSET_TYPE_STACKED = 2
} RRDSET_TYPE;

#define RRDSET_TYPE_LINE_NAME "line"
#define RRDSET_TYPE_AREA_NAME "area"
#define RRDSET_TYPE_STACKED_NAME "stacked"

RRDSET_TYPE rrdset_type_id(const char *name);
const char *rrdset_type_name(RRDSET_TYPE chart_type);


// ----------------------------------------------------------------------------
// memory mode

typedef enum rrd_memory_mode {
    RRD_MEMORY_MODE_NONE = 0,
    RRD_MEMORY_MODE_RAM  = 1,
    RRD_MEMORY_MODE_MAP  = 2,
    RRD_MEMORY_MODE_SAVE = 3,
    RRD_MEMORY_MODE_ALLOC = 4
} RRD_MEMORY_MODE;

#define RRD_MEMORY_MODE_NONE_NAME "none"
#define RRD_MEMORY_MODE_RAM_NAME "ram"
#define RRD_MEMORY_MODE_MAP_NAME "map"
#define RRD_MEMORY_MODE_SAVE_NAME "save"
#define RRD_MEMORY_MODE_ALLOC_NAME "alloc"

extern RRD_MEMORY_MODE default_rrd_memory_mode;

extern const char *rrd_memory_mode_name(RRD_MEMORY_MODE id);
extern RRD_MEMORY_MODE rrd_memory_mode_id(const char *name);


// ----------------------------------------------------------------------------
// algorithms types

typedef enum rrd_algorithm {
    RRD_ALGORITHM_ABSOLUTE              = 0,
    RRD_ALGORITHM_INCREMENTAL           = 1,
    RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL = 2,
    RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL  = 3
} RRD_ALGORITHM;

#define RRD_ALGORITHM_ABSOLUTE_NAME                "absolute"
#define RRD_ALGORITHM_INCREMENTAL_NAME             "incremental"
#define RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL_NAME   "percentage-of-incremental-row"
#define RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL_NAME    "percentage-of-absolute-row"

extern RRD_ALGORITHM rrd_algorithm_id(const char *name);
extern const char *rrd_algorithm_name(RRD_ALGORITHM algorithm);

// ----------------------------------------------------------------------------
// RRD FAMILY

struct rrdfamily {
    avl avl;

    const char *family;
    uint32_t hash_family;

    size_t use_count;

    avl_tree_lock rrdvar_root_index;
};
typedef struct rrdfamily RRDFAMILY;


// ----------------------------------------------------------------------------
// flags
// use this for configuration flags, not for state control
// flags are set/unset in a manner that is not thread safe
// and may lead to missing information.

typedef enum rrddim_flags {
    RRDDIM_FLAG_NONE                            = 0,
    RRDDIM_FLAG_HIDDEN                          = (1 << 0),  // this dimension will not be offered to callers
    RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS = (1 << 1)   // do not offer RESET or OVERFLOW info to callers
} RRDDIM_FLAGS;

#ifdef HAVE_C___ATOMIC
#define rrddim_flag_check(rd, flag) (__atomic_load_n(&((rd)->flags), __ATOMIC_SEQ_CST) & (flag))
#define rrddim_flag_set(rd, flag)   __atomic_or_fetch(&((rd)->flags), (flag), __ATOMIC_SEQ_CST)
#define rrddim_flag_clear(rd, flag) __atomic_and_fetch(&((rd)->flags), ~(flag), __ATOMIC_SEQ_CST)
#else
#define rrddim_flag_check(rd, flag) ((rd)->flags & (flag))
#define rrddim_flag_set(rd, flag)   (rd)->flags |= (flag)
#define rrddim_flag_clear(rd, flag) (rd)->flags &= ~(flag)
#endif


// ----------------------------------------------------------------------------
// RRD DIMENSION - this is a metric

struct rrddim {
    // ------------------------------------------------------------------------
    // binary indexing structures

    avl avl;                                        // the binary index - this has to be first member!

    // ------------------------------------------------------------------------
    // the dimension definition

    const char *id;                                 // the id of this dimension (for internal identification)
    const char *name;                               // the name of this dimension (as presented to user)
                                                    // this is a pointer to the config structure
                                                    // since the config always has a higher priority
                                                    // (the user overwrites the name of the charts)
                                                    // DO NOT FREE THIS - IT IS ALLOCATED IN CONFIG

    RRD_ALGORITHM algorithm;                        // the algorithm that is applied to add new collected values
    RRD_MEMORY_MODE rrd_memory_mode;                // the memory mode for this dimension

    collected_number multiplier;                    // the multiplier of the collected values
    collected_number divisor;                       // the divider of the collected values

    uint32_t flags;                                 // configuration flags for the dimension

    // ------------------------------------------------------------------------
    // members for temporary data we need for calculations

    uint32_t hash;                                  // a simple hash of the id, to speed up searching / indexing
                                                    // instead of strcmp() every item in the binary index
                                                    // we first compare the hashes

    uint32_t hash_name;                             // a simple hash of the name

    char *cache_filename;                           // the filename we load/save from/to this set

    size_t collections_counter;                     // the number of times we added values to this rrdim
    size_t unused[10];

    unsigned int updated:1;                         // 1 when the dimension has been updated since the last processing
    unsigned int exposed:1;                         // 1 when set what have sent this dimension to the central netdata

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

    size_t memsize;                                 // the memory allocated for this dimension

    char magic[sizeof(RRDDIMENSION_MAGIC) + 1];     // a string to be saved, used to identify our data file

    struct rrddimvar *variables;

    // ------------------------------------------------------------------------
    // the values stored in this dimension, using our floating point numbers

    storage_number values[];                        // the array of values - THIS HAS TO BE THE LAST MEMBER
};
typedef struct rrddim RRDDIM;

// ----------------------------------------------------------------------------
// these loop macros make sure the linked list is accessed with the right lock

#define rrddim_foreach_read(rd, st) \
    for((rd) = (st)->dimensions, rrdset_check_rdlock(st); (rd) ; (rd) = (rd)->next)

#define rrddim_foreach_write(rd, st) \
    for((rd) = (st)->dimensions, rrdset_check_wrlock(st); (rd) ; (rd) = (rd)->next)


// ----------------------------------------------------------------------------
// RRDSET - this is a chart

// use this for configuration flags, not for state control
// flags are set/unset in a manner that is not thread safe
// and may lead to missing information.

typedef enum rrdset_flags {
    RRDSET_FLAG_ENABLED          = 1 << 0, // enables or disables a chart
    RRDSET_FLAG_DETAIL           = 1 << 1, // if set, the data set should be considered as a detail of another
                                         // (the master data set should be the one that has the same family and is not detail)
    RRDSET_FLAG_DEBUG            = 1 << 2, // enables or disables debugging for a chart
    RRDSET_FLAG_OBSOLETE         = 1 << 3, // this is marked by the collector/module as obsolete
    RRDSET_FLAG_BACKEND_SEND     = 1 << 4, // if set, this chart should be sent to backends
    RRDSET_FLAG_BACKEND_IGNORE   = 1 << 5, // if set, this chart should not be sent to backends
    RRDSET_FLAG_EXPOSED_UPSTREAM = 1 << 6, // if set, we have sent this chart to netdata master (streaming)
    RRDSET_FLAG_STORE_FIRST      = 1 << 7, // if set, do not eliminate the first collection during interpolation
    RRDSET_FLAG_HETEROGENEOUS    = 1 << 8, // if set, the chart is not homogeneous (dimensions in it have multiple algorithms, multipliers or dividers)
    RRDSET_FLAG_HOMEGENEOUS_CHECK= 1 << 9, // if set, the chart should be checked to determine if the dimensions as homogeneous
    RRDSET_FLAG_HIDDEN           = 1 << 10, // if set, do not show this chart on the dashboard, but use it for backends
    RRDSET_FLAG_SYNC_CLOCK       = 1 << 11, // if set, microseconds on next data collection will be ignored (the chart will be synced to now)
} RRDSET_FLAGS;

#ifdef HAVE_C___ATOMIC
#define rrdset_flag_check(st, flag) (__atomic_load_n(&((st)->flags), __ATOMIC_SEQ_CST) & (flag))
#define rrdset_flag_set(st, flag)   __atomic_or_fetch(&((st)->flags), flag, __ATOMIC_SEQ_CST)
#define rrdset_flag_clear(st, flag) __atomic_and_fetch(&((st)->flags), ~flag, __ATOMIC_SEQ_CST)
#else
#define rrdset_flag_check(st, flag) ((st)->flags & (flag))
#define rrdset_flag_set(st, flag)   (st)->flags |= (flag)
#define rrdset_flag_clear(st, flag) (st)->flags &= ~(flag)
#endif
#define rrdset_flag_check_noatomic(st, flag) ((st)->flags & (flag))

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

    char *config_section;                           // the config section for the chart

    char *type;                                     // the type of graph RRD_TYPE_* (a category, for determining graphing options)
    char *family;                                   // grouping sets under the same family
    char *title;                                    // title shown to user
    char *units;                                    // units of measurement

    char *context;                                  // the template of this data set
    uint32_t hash_context;                          // the hash of the chart's context

    RRDSET_TYPE chart_type;                         // line, area, stacked

    int update_every;                               // every how many seconds is this updated?

    long entries;                                   // total number of entries in the data set

    long current_entry;                             // the entry that is currently being updated
                                                    // it goes around in a round-robin fashion

    uint32_t flags;                                 // configuration flags

    int gap_when_lost_iterations_above;             // after how many lost iterations a gap should be stored
                                                    // netdata will interpolate values for gaps lower than this

    long priority;                                  // the sorting priority of this chart


    // ------------------------------------------------------------------------
    // members for temporary data we need for calculations

    RRD_MEMORY_MODE rrd_memory_mode;                // if set to 1, this is memory mapped

    char *cache_dir;                                // the directory to store dimensions
    char cache_filename[FILENAME_MAX+1];            // the filename to store this set

    netdata_rwlock_t rrdset_rwlock;                 // protects dimensions linked list

    size_t counter;                                 // the number of times we added values to this database
    size_t counter_done;                            // the number of times rrdset_done() has been called

    time_t last_accessed_time;                      // the last time this RRDSET has been accessed
    time_t upstream_resync_time;                    // the timestamp up to which we should resync clock upstream

    char *plugin_name;                              // the name of the plugin that generated this
    char *module_name;                              // the name of the plugin module that generated this

    size_t unused[6];

    uint32_t hash;                                  // a simple hash on the id, to speed up searching
                                                    // we first compare hashes, and only if the hashes are equal we do string comparisons

    uint32_t hash_name;                             // a simple hash on the name

    usec_t usec_since_last_update;                  // the time in microseconds since the last collection of data

    struct timeval last_updated;                    // when this data set was last updated (updated every time the rrd_stats_done() function)
    struct timeval last_collected_time;             // when did this data set last collected values

    total_number collected_total;                   // used internally to calculate percentages
    total_number last_collected_total;              // used internally to calculate percentages

    RRDFAMILY *rrdfamily;                           // pointer to RRDFAMILY this chart belongs to
    RRDHOST *rrdhost;                               // pointer to RRDHOST this chart belongs to

    struct rrdset *next;                            // linking of rrdsets

    // ------------------------------------------------------------------------
    // local variables

    calculated_number green;                        // green threshold for this chart
    calculated_number red;                          // red threshold for this chart

    avl_tree_lock rrdvar_root_index;                // RRDVAR index for this chart
    RRDSETVAR *variables;                           // RRDSETVAR linked list for this chart (one RRDSETVAR, many RRDVARs)
    RRDCALC *alarms;                                // RRDCALC linked list for this chart

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

#define rrdset_rdlock(st) netdata_rwlock_rdlock(&((st)->rrdset_rwlock))
#define rrdset_wrlock(st) netdata_rwlock_wrlock(&((st)->rrdset_rwlock))
#define rrdset_unlock(st) netdata_rwlock_unlock(&((st)->rrdset_rwlock))


// ----------------------------------------------------------------------------
// these loop macros make sure the linked list is accessed with the right lock

#define rrdset_foreach_read(st, host) \
    for((st) = (host)->rrdset_root, rrdhost_check_rdlock(host); st ; (st) = (st)->next)

#define rrdset_foreach_write(st, host) \
    for((st) = (host)->rrdset_root, rrdhost_check_wrlock(host); st ; (st) = (st)->next)


// ----------------------------------------------------------------------------
// RRDHOST flags
// use this for configuration flags, not for state control
// flags are set/unset in a manner that is not thread safe
// and may lead to missing information.

typedef enum rrdhost_flags {
    RRDHOST_FLAG_ORPHAN                 = 1 << 0, // this host is orphan (not receiving data)
    RRDHOST_FLAG_DELETE_OBSOLETE_CHARTS = 1 << 1, // delete files of obsolete charts
    RRDHOST_FLAG_DELETE_ORPHAN_HOST     = 1 << 2, // delete the entire host when orphan
    RRDHOST_FLAG_BACKEND_SEND           = 1 << 3, // send it to backends
    RRDHOST_FLAG_BACKEND_DONT_SEND      = 1 << 4, // don't send it to backends
} RRDHOST_FLAGS;

#ifdef HAVE_C___ATOMIC
#define rrdhost_flag_check(host, flag) (__atomic_load_n(&((host)->flags), __ATOMIC_SEQ_CST) & (flag))
#define rrdhost_flag_set(host, flag)   __atomic_or_fetch(&((host)->flags), flag, __ATOMIC_SEQ_CST)
#define rrdhost_flag_clear(host, flag) __atomic_and_fetch(&((host)->flags), ~flag, __ATOMIC_SEQ_CST)
#else
#define rrdhost_flag_check(host, flag) ((host)->flags & (flag))
#define rrdhost_flag_set(host, flag)   (host)->flags |= (flag)
#define rrdhost_flag_clear(host, flag) (host)->flags &= ~(flag)
#endif

#ifdef NETDATA_INTERNAL_CHECKS
#define rrdset_debug(st, fmt, args...) do { if(unlikely(debug_flags & D_RRD_STATS && rrdset_flag_check(st, RRDSET_FLAG_DEBUG))) \
            debug_int(__FILE__, __FUNCTION__, __LINE__, "%s: " fmt, st->name, ##args); } while(0)
#else
#define rrdset_debug(st, fmt, args...) debug_dummy()
#endif


// ----------------------------------------------------------------------------
// RRD HOST

struct rrdhost {
    avl avl;                                        // the index of hosts

    // ------------------------------------------------------------------------
    // host information

    char *hostname;                                 // the hostname of this host
    uint32_t hash_hostname;                         // the hostname hash

    char *registry_hostname;                        // the registry hostname for this host

    char machine_guid[GUID_LEN + 1];                // the unique ID of this host
    uint32_t hash_machine_guid;                     // the hash of the unique ID

    const char *os;                                 // the O/S type of the host
    const char *tags;                               // tags for this host
    const char *timezone;                           // the timezone of the host

    uint32_t flags;                                 // flags about this RRDHOST

    int rrd_update_every;                           // the update frequency of the host
    long rrd_history_entries;                       // the number of history entries for the host's charts
    RRD_MEMORY_MODE rrd_memory_mode;                // the memory more for the charts of this host

    char *cache_dir;                                // the directory to save RRD cache files
    char *varlib_dir;                               // the directory to save health log

    char *program_name;                             // the program name that collects metrics for this host
    char *program_version;                          // the program version that collects metrics for this host

    // ------------------------------------------------------------------------
    // streaming of data to remote hosts - rrdpush

    unsigned int rrdpush_send_enabled:1;            // 1 when this host sends metrics to another netdata
    char *rrdpush_send_destination;                 // where to send metrics to
    char *rrdpush_send_api_key;                     // the api key at the receiving netdata

    // the following are state information for the threading
    // streaming metrics from this netdata to an upstream netdata
    volatile unsigned int rrdpush_sender_spawn:1;   // 1 when the sender thread has been spawn
    netdata_thread_t rrdpush_sender_thread;         // the sender thread

    volatile unsigned int rrdpush_sender_connected:1; // 1 when the sender is ready to push metrics
    int rrdpush_sender_socket;                      // the fd of the socket to the remote host, or -1

    volatile unsigned int rrdpush_sender_error_shown:1; // 1 when we have logged a communication error
    volatile unsigned int rrdpush_sender_join:1;    // 1 when we have to join the sending thread

    // metrics may be collected asynchronously
    // these synchronize all the threads willing the write to our sending buffer
    netdata_mutex_t rrdpush_sender_buffer_mutex;    // exclusive access to rrdpush_sender_buffer
    int rrdpush_sender_pipe[2];                     // collector to sender thread signaling
    BUFFER *rrdpush_sender_buffer;                  // collector fills it, sender sends it


    // ------------------------------------------------------------------------
    // streaming of data from remote hosts - rrdpush

    volatile size_t connected_senders;              // when remote hosts are streaming to this
                                                    // host, this is the counter of connected clients

    time_t senders_disconnected_time;               // the time the last sender was disconnected

    // ------------------------------------------------------------------------
    // health monitoring options

    unsigned int health_enabled:1;                  // 1 when this host has health enabled
    time_t health_delay_up_to;                      // a timestamp to delay alarms processing up to
    char *health_default_exec;                      // the full path of the alarms notifications program
    char *health_default_recipient;                 // the default recipient for all alarms
    char *health_log_filename;                      // the alarms event log filename
    size_t health_log_entries_written;              // the number of alarm events writtern to the alarms event log
    FILE *health_log_fp;                            // the FILE pointer to the open alarms event log file

    // all RRDCALCs are primarily allocated and linked here
    // RRDCALCs may be linked to charts at any point
    // (charts may or may not exist when these are loaded)
    RRDCALC *alarms;

    ALARM_LOG health_log;                           // alarms historical events (event log)
    uint32_t health_last_processed_id;              // the last processed health id from the log
    uint32_t health_max_unique_id;                  // the max alarm log unique id given for the host
    uint32_t health_max_alarm_id;                   // the max alarm id given for the host

    // templates of alarms
    // these are used to create alarms when charts
    // are created or renamed, that match them
    RRDCALCTEMPLATE *templates;


    // ------------------------------------------------------------------------
    // the charts of the host

    RRDSET *rrdset_root;                            // the host charts


    // ------------------------------------------------------------------------
    // locks

    netdata_rwlock_t rrdhost_rwlock;                // lock for this RRDHOST (protects rrdset_root linked list)

    // ------------------------------------------------------------------------
    // indexes

    avl_tree_lock rrdset_root_index;                // the host's charts index (by id)
    avl_tree_lock rrdset_root_index_name;           // the host's charts index (by name)

    avl_tree_lock rrdfamily_root_index;             // the host's chart families index
    avl_tree_lock rrdvar_root_index;                // the host's chart variables index

    struct rrdhost *next;
};
extern RRDHOST *localhost;

#define rrdhost_rdlock(host) netdata_rwlock_rdlock(&((host)->rrdhost_rwlock))
#define rrdhost_wrlock(host) netdata_rwlock_wrlock(&((host)->rrdhost_rwlock))
#define rrdhost_unlock(host) netdata_rwlock_unlock(&((host)->rrdhost_rwlock))

// ----------------------------------------------------------------------------
// these loop macros make sure the linked list is accessed with the right lock

#define rrdhost_foreach_read(var) \
    for((var) = localhost, rrd_check_rdlock(); var ; (var) = (var)->next)

#define rrdhost_foreach_write(var) \
    for((var) = localhost, rrd_check_wrlock(); var ; (var) = (var)->next)


// ----------------------------------------------------------------------------
// global lock for all RRDHOSTs

extern netdata_rwlock_t rrd_rwlock;

#define rrd_rdlock() netdata_rwlock_rdlock(&rrd_rwlock)
#define rrd_wrlock() netdata_rwlock_wrlock(&rrd_rwlock)
#define rrd_unlock() netdata_rwlock_unlock(&rrd_rwlock)

// ----------------------------------------------------------------------------

extern size_t rrd_hosts_available;
extern time_t rrdhost_free_orphan_time;

extern void rrd_init(char *hostname);

extern RRDHOST *rrdhost_find_by_hostname(const char *hostname, uint32_t hash);
extern RRDHOST *rrdhost_find_by_guid(const char *guid, uint32_t hash);

extern RRDHOST *rrdhost_find_or_create(
        const char *hostname
        , const char *registry_hostname
        , const char *guid
        , const char *os
        , const char *timezone
        , const char *tags
        , const char *program_name
        , const char *program_version
        , int update_every
        , long history
        , RRD_MEMORY_MODE mode
        , unsigned int health_enabled
        , unsigned int rrdpush_enabled
        , char *rrdpush_destination
        , char *rrdpush_api_key
);

#if defined(NETDATA_INTERNAL_CHECKS) && defined(NETDATA_VERIFY_LOCKS)
extern void __rrdhost_check_wrlock(RRDHOST *host, const char *file, const char *function, const unsigned long line);
extern void __rrdhost_check_rdlock(RRDHOST *host, const char *file, const char *function, const unsigned long line);
extern void __rrdset_check_rdlock(RRDSET *st, const char *file, const char *function, const unsigned long line);
extern void __rrdset_check_wrlock(RRDSET *st, const char *file, const char *function, const unsigned long line);
extern void __rrd_check_rdlock(const char *file, const char *function, const unsigned long line);
extern void __rrd_check_wrlock(const char *file, const char *function, const unsigned long line);

#define rrdhost_check_rdlock(host) __rrdhost_check_rdlock(host, __FILE__, __FUNCTION__, __LINE__)
#define rrdhost_check_wrlock(host) __rrdhost_check_wrlock(host, __FILE__, __FUNCTION__, __LINE__)
#define rrdset_check_rdlock(st) __rrdset_check_rdlock(st, __FILE__, __FUNCTION__, __LINE__)
#define rrdset_check_wrlock(st) __rrdset_check_wrlock(st, __FILE__, __FUNCTION__, __LINE__)
#define rrd_check_rdlock() __rrd_check_rdlock(__FILE__, __FUNCTION__, __LINE__)
#define rrd_check_wrlock() __rrd_check_wrlock(__FILE__, __FUNCTION__, __LINE__)

#else
#define rrdhost_check_rdlock(host) (void)0
#define rrdhost_check_wrlock(host) (void)0
#define rrdset_check_rdlock(st) (void)0
#define rrdset_check_wrlock(st) (void)0
#define rrd_check_rdlock() (void)0
#define rrd_check_wrlock() (void)0
#endif

// ----------------------------------------------------------------------------
// RRDSET functions

extern int rrdset_set_name(RRDSET *st, const char *name);

extern RRDSET *rrdset_create_custom(RRDHOST *host
                             , const char *type
                             , const char *id
                             , const char *name
                             , const char *family
                             , const char *context
                             , const char *title
                             , const char *units
                             , const char *plugin
                             , const char *module
                             , long priority
                             , int update_every
                             , RRDSET_TYPE chart_type
                             , RRD_MEMORY_MODE memory_mode
                             , long history_entries);

#define rrdset_create(host, type, id, name, family, context, title, units, plugin, module, priority, update_every, chart_type) \
    rrdset_create_custom(host, type, id, name, family, context, title, units, plugin, module, priority, update_every, chart_type, (host)->rrd_memory_mode, (host)->rrd_history_entries)

#define rrdset_create_localhost(type, id, name, family, context, title, units, plugin, module, priority, update_every, chart_type) \
    rrdset_create(localhost, type, id, name, family, context, title, units, plugin, module, priority, update_every, chart_type)

extern void rrdhost_free_all(void);
extern void rrdhost_save_all(void);
extern void rrdhost_cleanup_all(void);

extern void rrdhost_cleanup_orphan_hosts_nolock(RRDHOST *protected);
extern void rrdhost_free(RRDHOST *host);
extern void rrdhost_save_charts(RRDHOST *host);
extern void rrdhost_delete_charts(RRDHOST *host);

extern int rrdhost_should_be_removed(RRDHOST *host, RRDHOST *protected, time_t now);

extern void rrdset_update_heterogeneous_flag(RRDSET *st);

extern RRDSET *rrdset_find(RRDHOST *host, const char *id);
#define rrdset_find_localhost(id) rrdset_find(localhost, id)

extern RRDSET *rrdset_find_bytype(RRDHOST *host, const char *type, const char *id);
#define rrdset_find_bytype_localhost(type, id) rrdset_find_bytype(localhost, type, id)

extern RRDSET *rrdset_find_byname(RRDHOST *host, const char *name);
#define rrdset_find_byname_localhost(name)  rrdset_find_byname(localhost, name)

extern void rrdset_next_usec_unfiltered(RRDSET *st, usec_t microseconds);
extern void rrdset_next_usec(RRDSET *st, usec_t microseconds);
#define rrdset_next(st) rrdset_next_usec(st, 0ULL)

extern void rrdset_done(RRDSET *st);

extern void rrdset_is_obsolete(RRDSET *st);
extern void rrdset_isnot_obsolete(RRDSET *st);

// checks if the RRDSET should be offered to viewers
#define rrdset_is_available_for_viewers(st) (rrdset_flag_check(st, RRDSET_FLAG_ENABLED) && !rrdset_flag_check(st, RRDSET_FLAG_HIDDEN) && !rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE) && (st)->dimensions && (st)->rrd_memory_mode != RRD_MEMORY_MODE_NONE)
#define rrdset_is_available_for_backends(st) (rrdset_flag_check(st, RRDSET_FLAG_ENABLED) && !rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE) && (st)->dimensions)

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

extern RRDDIM *rrddim_add_custom(RRDSET *st, const char *id, const char *name, collected_number multiplier, collected_number divisor, RRD_ALGORITHM algorithm, RRD_MEMORY_MODE memory_mode);
#define rrddim_add(st, id, name, multiplier, divisor, algorithm) rrddim_add_custom(st, id, name, multiplier, divisor, algorithm, (st)->rrd_memory_mode)

extern int rrddim_set_name(RRDSET *st, RRDDIM *rd, const char *name);
extern int rrddim_set_algorithm(RRDSET *st, RRDDIM *rd, RRD_ALGORITHM algorithm);
extern int rrddim_set_multiplier(RRDSET *st, RRDDIM *rd, collected_number multiplier);
extern int rrddim_set_divisor(RRDSET *st, RRDDIM *rd, collected_number divisor);

extern RRDDIM *rrddim_find(RRDSET *st, const char *id);

extern int rrddim_hide(RRDSET *st, const char *id);
extern int rrddim_unhide(RRDSET *st, const char *id);

extern collected_number rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, collected_number value);
extern collected_number rrddim_set(RRDSET *st, const char *id, collected_number value);

extern long align_entries_to_pagesize(RRD_MEMORY_MODE mode, long entries);

// ----------------------------------------------------------------------------
// RRD internal functions

#ifdef NETDATA_RRD_INTERNALS

extern avl_tree_lock rrdhost_root_index;

extern char *rrdset_strncpyz_name(char *to, const char *from, size_t length);
extern char *rrdset_cache_dir(RRDHOST *host, const char *id, const char *config_section);

extern void rrddim_free(RRDSET *st, RRDDIM *rd);

extern int rrddim_compare(void* a, void* b);
extern int rrdset_compare(void* a, void* b);
extern int rrdset_compare_name(void* a, void* b);
extern int rrdfamily_compare(void *a, void *b);

extern RRDFAMILY *rrdfamily_create(RRDHOST *host, const char *id);
extern void rrdfamily_free(RRDHOST *host, RRDFAMILY *rc);

#define rrdset_index_add(host, st) (RRDSET *)avl_insert_lock(&((host)->rrdset_root_index), (avl *)(st))
#define rrdset_index_del(host, st) (RRDSET *)avl_remove_lock(&((host)->rrdset_root_index), (avl *)(st))
extern RRDSET *rrdset_index_del_name(RRDHOST *host, RRDSET *st);

extern void rrdset_free(RRDSET *st);
extern void rrdset_reset(RRDSET *st);
extern void rrdset_save(RRDSET *st);
extern void rrdset_delete(RRDSET *st);

extern void rrdhost_cleanup_obsolete_charts(RRDHOST *host);

#endif /* NETDATA_RRD_INTERNALS */


#endif /* NETDATA_RRD_H */
