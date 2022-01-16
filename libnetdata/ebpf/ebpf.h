// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_H
#define NETDATA_EBPF_H 1

#include <bpf/bpf.h>
#include <bpf/libbpf.h>
#include <stdlib.h> // Necessary for stdtoul

#define NETDATA_DEBUGFS "/sys/kernel/debug/tracing/"
#define NETDATA_KALLSYMS "/proc/kallsyms"

// Config files
#define EBPF_GLOBAL_SECTION "global"
#define EBPF_CFG_LOAD_MODE "ebpf load mode"
#define EBPF_CFG_LOAD_MODE_DEFAULT "entry"
#define EBPF_CFG_LOAD_MODE_RETURN "return"
#define EBPF_MAX_MODE_LENGTH 6

#define EBPF_CFG_UPDATE_EVERY "update every"
#define EBPF_CFG_PID_SIZE "pid table size"
#define EBPF_CFG_APPLICATION "apps"
#define EBPF_CFG_CGROUP "cgroups"

/**
 * The RedHat magic number was got doing:
 *
 * 1797 = 7*256 + 5
 *
 *  For more details, please, read /usr/include/linux/version.h
 *  in any Red Hat installation.
 */
#define NETDATA_MINIMUM_RH_VERSION 1797

/**
 * 2048 = 8*256 + 0
 */
#define NETDATA_RH_8 2048

/**
 *  Kernel Version
 *
 *  Kernel versions are calculated using the following formula:
 *
 *  VERSION = LINUX_VERSION_MAJOR*65536 + LINUX_VERSION_PATCHLEVEL*256 + LINUX_VERSION_SUBLEVEL
 *
 *  Where LINUX_VERSION_MAJOR, LINUX_VERSION_PATCHLEVEL, and LINUX_VERSION_SUBLEVEL are extracted
 *  from /usr/include/linux/version.h.
 *
 *  LINUX_VERSION_SUBLEVEL has the maximum value 255, but linux can have more SUBLEVELS.
 *
 */
enum netdata_ebpf_kernel_versions {
    NETDATA_EBPF_KERNEL_4_11 = 264960,  //  264960 = 4 * 65536 + 15 * 256
    NETDATA_EBPF_KERNEL_4_15 = 265984,  //  265984 = 4 * 65536 + 15 * 256
    NETDATA_EBPF_KERNEL_4_17 = 266496,  //  266496 = 4 * 65536 + 17 * 256
    NETDATA_EBPF_KERNEL_5_0  = 327680,  //  327680 = 5 * 65536 +  0 * 256
    NETDATA_EBPF_KERNEL_5_10 = 330240,  //  330240 = 5 * 65536 + 10 * 256
    NETDATA_EBPF_KERNEL_5_11 = 330496,  //  330240 = 5 * 65536 + 11 * 256
    NETDATA_EBPF_KERNEL_5_15 = 331520   //  331520 = 5 * 65536 + 15 * 256
};

enum netdata_kernel_flag {
    NETDATA_V3_10 = 1 << 0,
    NETDATA_V4_14 = 1 << 1,
    NETDATA_V4_16 = 1 << 2,
    NETDATA_V4_18 = 1 << 3,
    NETDATA_V5_4  = 1 << 4,
    NETDATA_V5_10 = 1 << 5,
    NETDATA_V5_11 = 1 << 6,
    NETDATA_V5_15 = 1 << 7
};

/**
 * Minimum value has relationship with libbpf support.
 */
#define NETDATA_MINIMUM_EBPF_KERNEL NETDATA_EBPF_KERNEL_4_11

#define VERSION_STRING_LEN 256
#define EBPF_KERNEL_REJECT_LIST_FILE "ebpf_kernel_reject_list.txt"

#define ND_EBPF_DEFAULT_MIN_PID 1U
#define ND_EBPF_MAP_FD_NOT_INITIALIZED (int)-1

typedef struct ebpf_addresses {
    char *function;
    uint32_t hash;
    // We use long as address, because it matches system length
    unsigned long addr;
} ebpf_addresses_t;

extern char *ebpf_user_config_dir;
extern char *ebpf_stock_config_dir;

typedef struct ebpf_data {
    int *map_fd;

    char *kernel_string;
    uint32_t running_on_kernel;
    int isrh;
} ebpf_data_t;

typedef enum {
    MODE_RETURN = 0, // This attaches kprobe when the function returns
    MODE_DEVMODE,    // This stores log given description about the errors raised
    MODE_ENTRY       // This attaches kprobe when the function is called
} netdata_run_mode_t;

#define ND_EBPF_DEFAULT_PID_SIZE 32768U

enum netdata_ebpf_map_type {
    NETDATA_EBPF_MAP_STATIC = 0,
    NETDATA_EBPF_MAP_RESIZABLE = 1,
    NETDATA_EBPF_MAP_CONTROLLER = 2,
    NETDATA_EBPF_MAP_CONTROLLER_UPDATED = 4,
    NETDATA_EBPF_MAP_PID = 8
};

enum netdata_controller {
    NETDATA_CONTROLLER_APPS_ENABLED,

    NETDATA_CONTROLLER_END
};

typedef struct ebpf_local_maps {
    char *name;
    uint32_t internal_input;
    uint32_t user_input;
    uint32_t type;
    int map_fd;
} ebpf_local_maps_t;

typedef struct ebpf_specify_name {
    char *program_name;
    char *function_to_attach;
    char *optional;
    bool retprobe;
} ebpf_specify_name_t;

typedef enum netdata_ebpf_load_mode {
    EBPF_LOAD_LEGACY,        // Select legacy mode, this means we will load binaries
    EBPF_LOAD_CORE,          // When CO-RE is used, it is necessary to use the souce code

    EBPF_LOAD_PLAY_DICE      // Take a look on environment and choose the best option
} netdata_ebpf_load_mode_t;

typedef enum netdata_ebpf_program_loaded {
    EBPF_LOAD_PROBE,         // Attach probes on targets
    EBPF_LOAD_RETPROBE,      // Attach retprobes on targets
    EBPF_LOAD_TRACEPOINT,    // This stores log given description about the errors raised
    EBPF_LOAD_TRAMPOLINE,    // This attaches kprobe when the function is called
} netdata_ebpf_program_loaded_t;

typedef struct netdata_ebpf_targets {
    char *name;
    netdata_ebpf_program_loaded_t mode;
} netdata_ebpf_targets_t;

typedef struct ebpf_plugin_stats {
    // Load options
    uint32_t legacy;      // Legacy codes
    uint32_t core;        // CO-RE codes, this means we are using source code compiled.

    uint32_t threads;     // Total number of threads
    uint32_t running;     // total number of threads running

    uint32_t probes;      // Number of kprobes loaded
    uint32_t retprobes;   // Number of kretprobes loaded
    uint32_t tracepoints; // Number of tracepoints used
    uint32_t trampolines; // Number of trampolines used
} ebpf_plugin_stats_t;

typedef struct ebpf_module {
    const char *thread_name;
    const char *config_name;
    int enabled;
    void *(*start_routine)(void *);
    int update_every;
    int global_charts;
    int apps_charts;
    int cgroup_charts;
    netdata_run_mode_t mode;
    uint32_t thread_id;
    int optional;
    void (*apps_routine)(struct ebpf_module *em, void *ptr);
    ebpf_local_maps_t *maps;
    ebpf_specify_name_t *names;
    uint32_t pid_map_size;
    struct config *cfg;
    const char *config_file;
    uint64_t kernels;
    netdata_ebpf_load_mode_t load;
    netdata_ebpf_targets_t *targets;
} ebpf_module_t;

extern int ebpf_get_kernel_version();
extern int get_redhat_release();
extern int has_condition_to_run(int version);
extern char *ebpf_kernel_suffix(int version, int isrh);
extern struct bpf_link **ebpf_load_program(char *plugins_dir, ebpf_module_t *em, int kver, int is_rhf,
                                           struct bpf_object **obj);

extern void ebpf_mount_config_name(char *filename, size_t length, char *path, const char *config);
extern int ebpf_load_config(struct config *config, char *filename);
extern void ebpf_update_module(ebpf_module_t *em);
extern void ebpf_update_names(ebpf_specify_name_t *opt, ebpf_module_t *em);
extern char *ebpf_find_symbol(char *search);
extern void ebpf_load_addresses(ebpf_addresses_t *fa, int fd);
extern void ebpf_fill_algorithms(int *algorithms, size_t length, int algorithm);
extern char **ebpf_fill_histogram_dimension(size_t maximum);
extern void ebpf_update_general_stats(ebpf_plugin_stats_t *report, ebpf_module_t *em, netdata_ebpf_load_mode_t loaded);

// Histogram
#define NETDATA_EBPF_HIST_MAX_BINS 24UL
#define NETDATA_DISK_MAX 256U
#define NETDATA_DISK_HISTOGRAM_LENGTH (NETDATA_DISK_MAX * NETDATA_EBPF_HIST_MAX_BINS)

typedef struct netdata_ebpf_histogram {
    char *name;
    char *title;
    int order;
    uint64_t histogram[NETDATA_EBPF_HIST_MAX_BINS];
} netdata_ebpf_histogram_t;

extern void ebpf_histogram_dimension_cleanup(char **ptr, size_t length);

// Tracepoint helpers
// For more information related to tracepoints read https://www.kernel.org/doc/html/latest/trace/tracepoints.html
extern int ebpf_is_tracepoint_enabled(char *subsys, char *eventname);
extern int ebpf_enable_tracing_values(char *subsys, char *eventname);
extern int ebpf_disable_tracing_values(char *subsys, char *eventname);

#endif /* NETDATA_EBPF_H */
