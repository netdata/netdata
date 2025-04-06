// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDHOST_H
#define NETDATA_RRDHOST_H

#include "libnetdata/libnetdata.h"

#define HOST_LABEL_IS_EPHEMERAL "_is_ephemeral"

struct stream_thread;
struct rrdset;

typedef struct rrdhost RRDHOST;
typedef struct ml_host rrd_ml_host_t;
typedef struct rrdhost_acquired RRDHOST_ACQUIRED;

//#include "streaming/stream-traffic-types.h"
#include "streaming/stream-sender-commit.h"
#include "rrd-database-mode.h"
//#include "streaming/stream-replication-tracking.h"
#include "streaming/stream-parents.h"
#include "streaming/stream-path.h"
#include "storage-engine.h"
//#include "streaming/stream-traffic-types.h"
#include "rrdlabels.h"
#include "health/health-alert-log.h"

// ----------------------------------------------------------------------------
// RRDHOST flags
// use this for configuration flags, not for state control
// flags are set/unset in a manner that is not thread safe
// and may lead to missing information.

typedef enum __attribute__ ((__packed__)) rrdhost_flags {

    // Careful not to overlap with rrdhost_options to avoid bugs if
    // rrdhost_flags_xxx is used instead of rrdhost_option_xxx or vice-versa
    // Orphan, Archived and Obsolete flags

    /*
     * 3 BASE FLAGS FOR HOSTS:
     *
     * - COLLECTOR_ONLINE = the collector is currently collecting data for this node
     *                      this is true FOR ALL KINDS OF NODES (including localhost, virtual hosts, children)
     *
     * - ORPHAN           = the node had a collector online recently, but does not have it now
     *
     * - ARCHIVED         = the node does not have data collection structures attached to it
     *
     */

    RRDHOST_FLAG_COLLECTOR_ONLINE               = (1 << 7), // the collector of this host is online
    RRDHOST_FLAG_ORPHAN                         = (1 << 8), // this host is orphan (not receiving data)
    RRDHOST_FLAG_ARCHIVED                       = (1 << 9), // The host is archived, no collected charts yet

    RRDHOST_FLAG_PENDING_OBSOLETE_CHARTS        = (1 << 10), // the host has pending chart obsoletions
    RRDHOST_FLAG_PENDING_OBSOLETE_DIMENSIONS    = (1 << 11), // the host has pending dimension obsoletions

    // Streaming sender
    RRDHOST_FLAG_STREAM_SENDER_INITIALIZED      = (1 << 12), // the host has initialized streaming sender structures
    RRDHOST_FLAG_STREAM_SENDER_ADDED            = (1 << 13), // When set, the sender thread is running
    RRDHOST_FLAG_STREAM_SENDER_CONNECTED        = (1 << 14), // When set, the host is connected to a parent
    RRDHOST_FLAG_STREAM_SENDER_READY_4_METRICS  = (1 << 15), // when set, rrdset_done() should push metrics to parent
    RRDHOST_FLAG_STREAM_SENDER_LOGGED_STATUS    = (1 << 16), // when set, we have logged the status of metrics streaming

    // Health
    RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION  = (1 << 17), // contains charts and dims with uninitialized variables
    RRDHOST_FLAG_INITIALIZED_HEALTH             = (1 << 18), // the host has initialized health structures

    // Exporting
    RRDHOST_FLAG_EXPORTING_SEND                 = (1 << 19), // send it to external databases
    RRDHOST_FLAG_EXPORTING_DONT_SEND            = (1 << 20), // don't send it to external databases

    // ACLK
    RRDHOST_FLAG_ACLK_STREAM_CONTEXTS           = (1 << 21), // when set, we should send ACLK stream context updates
    RRDHOST_FLAG_ACLK_STREAM_ALERTS             = (1 << 22), // Host should stream alerts

    // Metadata
    RRDHOST_FLAG_METADATA_UPDATE                = (1 << 23), // metadata needs to be stored in the database
    RRDHOST_FLAG_METADATA_LABELS                = (1 << 24), // metadata needs to be stored in the database
    RRDHOST_FLAG_METADATA_INFO                  = (1 << 25), // metadata needs to be stored in the database
    RRDHOST_FLAG_PENDING_CONTEXT_LOAD           = (1 << 26), // Context needs to be loaded

    RRDHOST_FLAG_METADATA_CLAIMID               = (1 << 27), // metadata needs to be stored in the database

    RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED       = (1 << 28), // set when the host has updated global functions
    RRDHOST_FLAG_RRDCONTEXT_GET_RETENTION       = (1 << 29), // set when rrdcontext needs to update the retention of the host
} RRDHOST_FLAGS;

#define rrdhost_flag_get(host)                         atomic_flags_get(&((host)->flags))
#define rrdhost_flag_check(host, flag)                 atomic_flags_check(&((host)->flags), flag)
#define rrdhost_flag_set(host, flag)                   atomic_flags_set(&((host)->flags), flag)
#define rrdhost_flag_clear(host, flag)                 atomic_flags_clear(&((host)->flags), flag)
#define rrdhost_flag_set_and_clear(host, set, clear)   atomic_flags_set_and_clear(&((host)->flags), set, clear)

typedef enum __attribute__ ((__packed__)) {
    // Streaming configuration
    RRDHOST_OPTION_SENDER_ENABLED           = (1 << 0), // set when the host is configured to send metrics to a parent
    RRDHOST_OPTION_REPLICATION              = (1 << 1), // when set, we support replication for this host

    // Other options
    RRDHOST_OPTION_VIRTUAL_HOST             = (1 << 2), // when set, this host is a virtual one
    RRDHOST_OPTION_EPHEMERAL_HOST           = (1 << 3), // when set, this host is an ephemeral one
} RRDHOST_OPTIONS;

#define rrdhost_option_check(host, flag) ((host)->options & (flag))
#define rrdhost_option_set(host, flag)   (host)->options |= flag
#define rrdhost_option_clear(host, flag) (host)->options &= ~(flag)

#define rrdhost_has_stream_sender_enabled(host) (rrdhost_option_check(host, RRDHOST_OPTION_SENDER_ENABLED) && (host)->sender)

#define rrdhost_can_stream_metadata_to_parent(host)                                 \
    (rrdhost_has_stream_sender_enabled(host) &&                                     \
     rrdhost_flag_check(host, RRDHOST_FLAG_STREAM_SENDER_READY_4_METRICS) &&        \
     rrdhost_flag_check(host, RRDHOST_FLAG_COLLECTOR_ONLINE)                  \
    )


struct rrdhost {
    char machine_guid[GUID_LEN + 1];                // the unique ID of this host

    // ------------------------------------------------------------------------
    // host information

    STRING *hostname;                               // the hostname of this host
    STRING *registry_hostname;                      // the registry hostname for this host
    STRING *os;                                     // the O/S type of the host
    STRING *timezone;                               // the timezone of the host
    STRING *abbrev_timezone;                        // the abbriviated timezone of the host
    STRING *program_name;                           // the program name that collects metrics for this host
    STRING *program_version;                        // the program version that collects metrics for this host

    OBJECT_STATE state_id;                          // every time data collection (stream receiver) (dis)connects,
                           // this gets incremented - it is used to detect stale functions,
                           // stale backfilling requests, etc.

    int32_t utc_offset;                             // the offset in seconds from utc

    RRDHOST_OPTIONS options;                        // configuration option for this RRDHOST (no atomics on this)
    RRDHOST_FLAGS flags;                            // runtime flags about this RRDHOST (atomics on this)
    RRDHOST_FLAGS *exporting_flags;                 // array of flags for exporting connector instances

    int32_t rrd_update_every;                       // the update frequency of the host
    int32_t rrd_history_entries;                    // the number of history entries for the host's charts

    RRD_DB_MODE rrd_memory_mode;                    // the configured memory more for the charts of this host
                                                    // the actual per tier is at .db[tier].mode

    char *cache_dir;                                // the directory to save RRD cache files

    struct {
        RRD_DB_MODE mode;                           // the db mode for this tier
        STORAGE_ENGINE *eng;                        // the storage engine API for this tier
        STORAGE_INSTANCE *si;                       // the db instance for this tier
        uint32_t tier_grouping;                     // tier 0 iterations aggregated on this tier
    } db[RRD_STORAGE_TIERS];

    struct rrdhost_system_info *system_info;        // information collected from the host environment

    // ------------------------------------------------------------------------
    // streaming and replication, configuration and status

    struct {
        struct stream_thread *thread;
        uint8_t refcount;

        // --- sender ---

        struct {
            struct {
                struct {
                    SPINLOCK spinlock;

                    bool ignore;                    // when set, freeing slots will not put them in the available
                    uint32_t used;
                    uint32_t size;
                    uint32_t *array;
                } available;                        // keep track of the available chart slots per host

                uint32_t last_used;                 // the last slot we used for a chart (increments only)
            } pluginsd_chart_slots;

            struct {
                pid_t tid;

                time_t last_connected;              // last time child connected (stored in db)
                uint32_t connections;               // the number of times this sender has connected
                STREAM_HANDSHAKE reason;            // the last receiver exit reason

                struct {
                    uint32_t counter_in;            // counts the number of replication statements we have received
                    uint32_t counter_out;           // counts the number of replication statements we have sent
                    uint32_t charts;                // the number of charts currently being replicated to a parent
                } replication;
            } status;

            // reserved for the receiver/sender thread - do not use for other purposes
            struct sender_buffer commit;

            STRING *destination;                    // where to send metrics to
            STRING *api_key;                        // the api key at the receiving netdata
            SIMPLE_PATTERN *charts_matching;        // pattern to match the charts to be sent
            RRDHOST_STREAM_PARENTS parents;         // the list of parents (extracted from destination)
        } snd;

        // --- receiver ---

        struct {
            struct {
                SPINLOCK spinlock;                  // lock for the management of the allocation
                uint32_t size;
                struct rrdset **array;
            } pluginsd_chart_slots;

            struct {
                pid_t tid;

                time_t last_connected;              // the time the last sender was connected
                time_t last_disconnected;           // the time the last sender was disconnected
                time_t last_chart;                  // the time of the last CHART streaming command
                bool check_obsolete;                // set when child connects, will instruct parent to
                                                    // trigger a check for obsoleted charts since previous connect

                uint32_t connections;               // the number of times this receiver has connected
                STREAM_HANDSHAKE reason;            // the last receiver exit reason

                struct {
                    uint32_t counter_in;            // counts the number of replication statements we have received
                    uint32_t counter_out;           // counts the number of replication statements we have sent
                    uint32_t backfill_pending;      // the number of replication requests pending on us
                    uint32_t charts;                // the number of charts currently being replicated from a child
                    NETDATA_DOUBLE percent;         // the % of replication completion
                } replication;
            } status;
        } rcv;

        // --- configuration ---

        struct {
            time_t period;                          // max time we want to replicate from the child
            time_t step;                            // seconds per replication step
        } replication;

        RRDHOST_STREAM_PATH path;
    } stream;

    // the following are state information for the threading
    // streaming metrics from this netdata to an upstream netdata
    struct sender_state *sender;

    struct receiver_state *receiver;
    SPINLOCK receiver_lock;

    // ------------------------------------------------------------------------

    struct aclk_sync_cfg_t *aclk_config;

    // ------------------------------------------------------------------------
    // health monitoring options

    // health variables
    HEALTH health;

    // all RRDCALCs are primarily allocated and linked here
    DICTIONARY *rrdcalc_root_index;

    ALARM_LOG health_log;                           // alarms historical events (event log)
    uint32_t health_last_processed_id;              // the last processed health id from the log
    uint32_t health_max_unique_id;                  // the max alarm log unique id given for the host
    uint32_t health_max_alarm_id;                   // the max alarm id given for the host
    size_t health_transitions;                      // the number of times an alert changed state

    // ------------------------------------------------------------------------
    // locks

    SPINLOCK rrdhost_update_lock;

    // ------------------------------------------------------------------------
    // ML handle
    rrd_ml_host_t *ml_host;

    // ------------------------------------------------------------------------
    // Support for host-level labels
    RRDLABELS *rrdlabels;

    // ------------------------------------------------------------------------
    // Support for functions
    DICTIONARY *functions;                          // collector functions this rrdset supports, can be NULL

    // ------------------------------------------------------------------------
    // indexes

    DICTIONARY *rrdset_root_index;                  // the host's charts index (by id)
    DICTIONARY *rrdset_root_index_name;             // the host's charts index (by name)

    DICTIONARY *rrdvars;                            // the host's chart variables index
                         // this includes custom host variables

    struct {
        uint32_t metrics_count;                     // atomic
        uint32_t instances_count;                   // atomic
        uint32_t contexts_count;                    // atomic
    } collected;

    struct {
        DICTIONARY *contexts;
        DICTIONARY *hub_queue;
        DICTIONARY *pp_queue;
        uint32_t metrics_count;                     // atomic
        uint32_t instances_count;                   // atomic
        uint32_t contexts_count;                    // atomic
    } rrdctx;

    struct {
        SPINLOCK spinlock;
        time_t first_time_s;
        time_t last_time_s;
    } retention;

    ND_UUID host_id;                                // Global GUID for this host
    ND_UUID node_id;                                // Cloud node_id

    struct {
        ND_UUID claim_id_of_origin;
        ND_UUID claim_id_of_parent;
    } aclk;

    struct rrdhost *next;
    struct rrdhost *prev;
};

extern RRDHOST *localhost;

#define rrdhost_receiver_lock(host) spinlock_lock(&(host)->receiver_lock)
#define rrdhost_receiver_unlock(host) spinlock_unlock(&(host)->receiver_lock)

#define rrdhost_hostname(host) string2str((host)->hostname)
#define rrdhost_registry_hostname(host) string2str((host)->registry_hostname)
#define rrdhost_os(host) string2str((host)->os)
#define rrdhost_timezone(host) string2str((host)->timezone)
#define rrdhost_abbrev_timezone(host) string2str((host)->abbrev_timezone)
#define rrdhost_program_name(host) string2str((host)->program_name)
#define rrdhost_program_version(host) string2str((host)->program_version)

#define rrdhost_receiver_replicating_charts(host) (__atomic_load_n(&((host)->stream.rcv.status.replication.charts), __ATOMIC_RELAXED))
#define rrdhost_receiver_replicating_charts_plus_one(host) (__atomic_add_fetch(&((host)->stream.rcv.status.replication.charts), 1, __ATOMIC_RELAXED))
#define rrdhost_receiver_replicating_charts_minus_one(host) (__atomic_sub_fetch(&((host)->stream.rcv.status.replication.charts), 1, __ATOMIC_RELAXED))
#define rrdhost_receiver_replicating_charts_zero(host) (__atomic_store_n(&((host)->stream.rcv.status.replication.charts), 0, __ATOMIC_RELAXED))

#define rrdhost_sender_replicating_charts(host) (__atomic_load_n(&((host)->stream.snd.status.replication.charts), __ATOMIC_RELAXED))
#define rrdhost_sender_replicating_charts_plus_one(host) (__atomic_add_fetch(&((host)->stream.snd.status.replication.charts), 1, __ATOMIC_RELAXED))
#define rrdhost_sender_replicating_charts_minus_one(host) (__atomic_sub_fetch(&((host)->stream.snd.status.replication.charts), 1, __ATOMIC_RELAXED))
#define rrdhost_sender_replicating_charts_zero(host) (__atomic_store_n(&((host)->stream.snd.status.replication.charts), 0, __ATOMIC_RELAXED))

#define rrdhost_is_local(host)  ( \
    (host) == localhost ||                                                                                      \
    rrdhost_option_check(host, RRDHOST_OPTION_VIRTUAL_HOST)                                                     \
    )

#define rrdhost_is_online_flags(flags) ((flags & RRDHOST_FLAG_COLLECTOR_ONLINE) && !(flags & RRDHOST_FLAG_ORPHAN))

static inline bool rrdhost_is_online(RRDHOST *host) {
    if(rrdhost_is_local(host))
        return true;

    RRDHOST_FLAGS flags = rrdhost_flag_get(host);
    return rrdhost_is_online_flags(flags);
}

bool rrdhost_matches_window(RRDHOST *host, time_t after, time_t before, time_t now);

extern DICTIONARY *rrdhost_root_index;
size_t rrdhost_hosts_available(void);

RRDHOST_ACQUIRED *rrdhost_find_and_acquire(const char *machine_guid);
RRDHOST *rrdhost_acquired_to_rrdhost(RRDHOST_ACQUIRED *rha);
void rrdhost_acquired_release(RRDHOST_ACQUIRED *rha);

#define rrdhost_foreach_read(var) \
    for((var) = localhost; var ; (var) = (var)->next)

#define rrdhost_foreach_write(var) \
    for((var) = localhost; var ; (var) = (var)->next)

RRDHOST *rrdhost_find_by_hostname(const char *hostname);
RRDHOST *rrdhost_find_by_guid(const char *guid);
RRDHOST *rrdhost_find_by_node_id(char *node_id);

#ifdef RRDHOST_INTERNALS
RRDHOST *rrdhost_create(
    const char *hostname,
    const char *registry_hostname,
    const char *guid,
    const char *os,
    const char *timezone,
    const char *abbrev_timezone,
    int32_t utc_offset,
    const char *prog_name,
    const char *prog_version,
    int update_every,
    long entries,
    RRD_DB_MODE memory_mode,
    bool health,
    bool stream,
    STRING *parents,
    STRING *api_key,
    STRING *send_charts_matching,
    bool replication,
    time_t replication_period,
    time_t replication_step,
    struct rrdhost_system_info *system_info,
    int is_localhost,
    bool archived
    );

void rrdhost_init(void);
#endif

RRDHOST *rrdhost_find_or_create(
    const char *hostname,
    const char *registry_hostname,
    const char *guid,
    const char *os,
    const char *timezone,
    const char *abbrev_timezone,
    int32_t utc_offset,
    const char *prog_name,
    const char *prog_version,
    int update_every,
    long history,
    RRD_DB_MODE mode,
    bool health,
    bool stream,
    STRING *parents,
    STRING *api_key,
    STRING *send_charts_matching,
    bool replication,
    time_t replication_period,
    time_t replication_step,
    struct rrdhost_system_info *system_info,
    bool is_archived);

void rrdhost_free_all(void);

void rrdhost_free___while_having_rrd_wrlock(RRDHOST *host);
void rrdhost_cleanup_data_collection_and_health(RRDHOST *host);

bool rrdhost_should_be_cleaned_up(RRDHOST *host, RRDHOST *protected_host, time_t now_s);
bool rrdhost_should_run_health(RRDHOST *host);

void set_host_properties(
    RRDHOST *host, int update_every,
    RRD_DB_MODE memory_mode, const char *registry_hostname,
    const char *os, const char *tzone, const char *abbrev_tzone, int32_t utc_offset,
    const char *prog_name, const char *prog_version);

static inline void rrdhost_retention(RRDHOST *host, time_t now, bool online, time_t *from, time_t *to) {
    time_t first_time_s = 0, last_time_s = 0;
    spinlock_lock(&host->retention.spinlock);
    first_time_s = host->retention.first_time_s;
    last_time_s = host->retention.last_time_s;
    spinlock_unlock(&host->retention.spinlock);

    if(from)
        *from = first_time_s;

    if(to)
        *to = online ? now : last_time_s;
}

extern time_t rrdhost_cleanup_orphan_to_archive_time_s;
extern time_t rrdhost_free_ephemeral_time_s;

#include "rrdhost-collection.h"
#include "rrdhost-slots.h"
#include "rrdhost-labels.h"

#endif //NETDATA_RRDHOST_H
