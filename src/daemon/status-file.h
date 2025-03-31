// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STATUS_FILE_H
#define NETDATA_STATUS_FILE_H

#include "libnetdata/libnetdata.h"
#include "daemon/config/netdata-conf-profile.h"
#include "database/rrd-database-mode.h"
#include "claim/cloud-status.h"
#include "machine-guid.h"

#define STATUS_FILE_VERSION 24

typedef enum {
    DAEMON_STATUS_NONE,
    DAEMON_STATUS_INITIALIZING,
    DAEMON_STATUS_RUNNING,
    DAEMON_STATUS_EXITING,
    DAEMON_STATUS_EXITED,
} DAEMON_STATUS;
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(DAEMON_STATUS);

typedef enum {
    DAEMON_OS_TYPE_UNKNOWN,
    DAEMON_OS_TYPE_LINUX,
    DAEMON_OS_TYPE_FREEBSD,
    DAEMON_OS_TYPE_MACOS,
    DAEMON_OS_TYPE_WINDOWS,
} DAEMON_OS_TYPE;
ENUM_STR_DEFINE_FUNCTIONS_EXTERN(DAEMON_OS_TYPE);

typedef struct daemon_status_file {
    SPINLOCK spinlock;
    uint32_t v;                 // the version of the status file

    char version[32];           // the netdata version
    DAEMON_STATUS status;       // the daemon status
    EXIT_REASON exit_reason;    // the exit reason (maybe empty)
    ND_PROFILE profile;         // the profile of the agent
    DAEMON_OS_TYPE os_type;
    RRD_DB_MODE db_mode;
    CLOUD_STATUS cloud_status;
    uint8_t db_tiers;
    bool kubernetes;
    bool sentry_available;      // true when sentry support is compiled in

    time_t boottime;            // system boottime
    time_t uptime;              // netdata uptime
    usec_t timestamp_ut;        // the timestamp of the status file
    size_t restarts;            // the number of times this agent has restarted (ever)
    size_t crashes;             // the number of times this agent has crashed (ever)
    size_t posts;               // the number of posts to the backend
    ssize_t reliability;        // consecutive restarts: > 0 reliable, < 0 crashing

    ND_MACHINE_GUID host_id;    // the machine guid of the system

    ND_UUID boot_id;            // the boot id of the system
    ND_UUID invocation;         // the netdata invocation id generated the file
    ND_UUID node_id;            // the Netdata Cloud node id of the agent
    ND_UUID claim_id;           // the Netdata Cloud claim id of the agent
    ND_UUID machine_id;         // the unique machine id of the system

    struct {
        usec_t init_started_ut;
        time_t init;
        usec_t exit_started_ut;
        time_t exit;
    } timings;

    uint64_t oom_protection;
    uint64_t netdata_max_rss;
    OS_SYSTEM_MEMORY memory;
    OS_SYSTEM_DISK_SPACE var_cache;

    char install_type[32];
    char architecture[32];   // ECS: host.architecture
    char virtualization[32];
    char container[32];
    char kernel_version[32]; // ECS: os.kernel
    char os_name[32];        // ECS: os.name
    char os_version[32];     // ECS: os.version
    char os_id[64];          // ECS: os.family
    char os_id_like[64];     // ECS: os.platform
    char timezone[32];
    char cloud_provider_type[32];
    char cloud_instance_type[32];
    char cloud_instance_region[32];
    bool read_system_info;

    char stack_traces[63];   // the backend for capturing stack traces

    struct {
        SPINLOCK spinlock;
        long line;
        char filename[256];
        char function[128];
        char errno_str[64];
        char message[512];
        char stack_trace[4096];
        char thread[ND_THREAD_TAG_MAX + 1];
        pid_t thread_id;
        SIGNAL_CODE signal_code;
        uintptr_t fault_address;
        uint32_t worker_job_id;
        bool sentry;        // true when the error was also reported to sentry
    } fatal;
} DAEMON_STATUS_FILE;

// these are used instead of locks when locks cannot be used (signal handler, out of memory, etc)
#define dsf_acquire(ds) __atomic_load_n(&(ds).v, __ATOMIC_ACQUIRE)
#define dsf_release(ds) __atomic_store_n(&(ds).v, (ds).v, __ATOMIC_RELEASE)

// saves the current status
void daemon_status_file_update_status(DAEMON_STATUS status);

// returns true when the event is duplicate and should not be reported again
bool daemon_status_file_deadly_signal_received(EXIT_REASON reason, SIGNAL_CODE code, void *fault_address, bool chained_handler);

// check for a crash
void daemon_status_file_check_crash(void);

bool daemon_status_file_has_last_crashed(DAEMON_STATUS_FILE *ds);
bool daemon_status_file_was_incomplete_shutdown(void);

void daemon_status_file_startup_step(const char *step);
void daemon_status_file_shutdown_step(const char *step);
void daemon_status_file_shutdown_timeout(BUFFER *trace);

void daemon_status_file_init(void);
void daemon_status_file_register_fatal(const char *filename, const char *function, const char *message, const char *errno_str, const char *stack_trace, long line);

const char *daemon_status_file_get_install_type(void);
const char *daemon_status_file_get_architecture(void);
const char *daemon_status_file_get_virtualization(void);
const char *daemon_status_file_get_container(void);
const char *daemon_status_file_get_os_name(void);
const char *daemon_status_file_get_os_version(void);
const char *daemon_status_file_get_os_id(void);
const char *daemon_status_file_get_os_id_like(void);
const char *daemon_status_file_get_timezone(void);
const char *daemon_status_file_get_cloud_provider_type(void);
const char *daemon_status_file_get_cloud_instance_type(void);
const char *daemon_status_file_get_cloud_instance_region(void);

    const char *daemon_status_file_get_fatal_filename(void);
const char *daemon_status_file_get_fatal_function(void);
const char *daemon_status_file_get_fatal_message(void);
const char *daemon_status_file_get_fatal_errno(void);
const char *daemon_status_file_get_fatal_stack_trace(void);
const char *daemon_status_file_get_fatal_thread(void);
const char *daemon_status_file_get_stack_trace_backend(void);
pid_t daemon_status_file_get_fatal_thread_id(void);
long daemon_status_file_get_fatal_line(void);
DAEMON_STATUS daemon_status_file_get_status(void);
size_t daemon_status_file_get_restarts(void);
ssize_t daemon_status_file_get_reliability(void);
ND_MACHINE_GUID daemon_status_file_get_host_id(void);
size_t daemon_status_file_get_fatal_worker_job_id(void);

#endif //NETDATA_STATUS_FILE_H
