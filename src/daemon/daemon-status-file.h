// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DAEMON_STATUS_FILE_H
#define NETDATA_DAEMON_STATUS_FILE_H

#include "libnetdata/libnetdata.h"
#include "daemon/config/netdata-conf-profile.h"

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

    time_t boottime;            // system boottime
    time_t uptime;              // netdata uptime
    usec_t timestamp_ut;        // the timestamp of the status file
    size_t restarts;            // the number of times this agent has restarted

    ND_UUID boot_id;            // the boot id of the system
    ND_UUID invocation;         // the netdata invocation id generated the file
    ND_UUID host_id;            // the machine guid of the agent
    ND_UUID node_id;            // the Netdata Cloud node id of the agent
    ND_UUID claim_id;           // the Netdata Cloud claim id of the agent

    struct {
        usec_t init_started_ut;
        time_t init;
        usec_t exit_started_ut;
        time_t exit;
    } timings;

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
    bool read_system_info;

    struct {
        SPINLOCK spinlock;
        long line;
        char filename[256];
        char function[128];
        char errno_str[64];
        char message[512];
        char stack_trace[2048];
        char thread[ND_THREAD_TAG_MAX + 1];
    } fatal;

    struct {
        SPINLOCK spinlock;
        struct {
            XXH64_hash_t hash;
            usec_t timestamp_ut;
        } slot[10];
    } dedup;
} DAEMON_STATUS_FILE;

// saves the current status
void daemon_status_file_update_status(DAEMON_STATUS status);
void daemon_status_file_deadly_signal_received(EXIT_REASON reason);

// check for a crash
void daemon_status_file_check_crash(void);

bool daemon_status_file_has_last_crashed(void);
bool daemon_status_file_was_incomplete_shutdown(void);

void daemon_status_file_startup_step(const char *step);
void daemon_status_file_shutdown_step(const char *step);

void daemon_status_file_register_fatal(const char *filename, const char *function, const char *message, const char *errno_str, const char *stack_trace, long line);

#endif //NETDATA_DAEMON_STATUS_FILE_H
