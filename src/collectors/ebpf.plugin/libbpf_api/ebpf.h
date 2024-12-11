// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_H
#define NETDATA_EBPF_H 1

#define NETDATA_EBPF_PLUGIN_NAME "ebpf.plugin"

#include <bpf/bpf.h>
#include <bpf/libbpf.h>
#ifdef LIBBPF_DEPRECATED
#include <bpf/btf.h>
#include <linux/btf.h>
#endif
#include <stdlib.h> // Necessary for stdtoul
#include "libnetdata/aral/aral.h"

#define NETDATA_DEBUGFS "/sys/kernel/debug/tracing/"
#define NETDATA_KALLSYMS "/proc/kallsyms"

// Config files
#define EBPF_GLOBAL_SECTION "global"
#define EBPF_CFG_LOAD_MODE "ebpf load mode"
#define EBPF_CFG_LOAD_MODE_DEFAULT "entry"
#define EBPF_CFG_LOAD_MODE_RETURN "return"
#define EBPF_MAX_MODE_LENGTH 6

#define EBPF_CFG_TYPE_FORMAT "ebpf type format"
#define EBPF_CFG_DEFAULT_PROGRAM "auto"
#define EBPF_CFG_CORE_PROGRAM "CO-RE"
#define EBPF_CFG_LEGACY_PROGRAM "legacy"

#define EBPF_CFG_COLLECT_PID "collect pid"
#define EBPF_CFG_PID_REAL_PARENT "real parent"
#define EBPF_CFG_PID_PARENT "parent"
#define EBPF_CFG_PID_ALL "all"
#define EBPF_CFG_PID_INTERNAL_USAGE "not used"

#define EBPF_CFG_CORE_ATTACH "ebpf co-re tracing"
#define EBPF_CFG_ATTACH_TRAMPOLINE "trampoline"
#define EBPF_CFG_ATTACH_TRACEPOINT "tracepoint"
#define EBPF_CFG_ATTACH_PROBE "probe"

#define EBPF_CFG_PROGRAM_PATH "btf path"

#define EBPF_CFG_MAPS_PER_CORE "maps per core"

#define EBPF_CFG_UPDATE_EVERY "update every"
#define EBPF_CFG_LIFETIME "lifetime"
#define EBPF_CFG_UPDATE_APPS_EVERY_DEFAULT 10
#define EBPF_CFG_PID_SIZE "pid table size"
#define EBPF_CFG_APPLICATION "apps"
#define EBPF_CFG_CGROUP "cgroups"

#define EBPF_COMMON_FNCT_CLEAN_UP "release_task"

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
    NETDATA_EBPF_KERNEL_4_06 = 263680,  //  264960 = 4 * 65536 +  6 * 256
    NETDATA_EBPF_KERNEL_4_11 = 264960,  //  264960 = 4 * 65536 + 15 * 256
    NETDATA_EBPF_KERNEL_4_14 = 265728,  //  264960 = 4 * 65536 + 14 * 256
    NETDATA_EBPF_KERNEL_4_15 = 265984,  //  265984 = 4 * 65536 + 15 * 256
    NETDATA_EBPF_KERNEL_4_17 = 266496,  //  266496 = 4 * 65536 + 17 * 256
    NETDATA_EBPF_KERNEL_5_0  = 327680,  //  327680 = 5 * 65536 +  0 * 256
    NETDATA_EBPF_KERNEL_5_4  = 328704,  //  327680 = 5 * 65536 +  4 * 256
    NETDATA_EBPF_KERNEL_5_5  = 328960,  //  327680 = 5 * 65536 +  5 * 256
    NETDATA_EBPF_KERNEL_5_10 = 330240,  //  330240 = 5 * 65536 + 10 * 256
    NETDATA_EBPF_KERNEL_5_11 = 330496,  //  330240 = 5 * 65536 + 11 * 256
    NETDATA_EBPF_KERNEL_5_14 = 331264,  //  331264 = 5 * 65536 + 14 * 256
    NETDATA_EBPF_KERNEL_5_15 = 331520,  //  331520 = 5 * 65536 + 15 * 256
    NETDATA_EBPF_KERNEL_5_16 = 331776,  //  331776 = 5 * 65536 + 16 * 256
    NETDATA_EBPF_KERNEL_6_8  = 395264   //  395264 = 5 * 65536 +  8 * 256
};

enum netdata_kernel_flag {
    NETDATA_V3_10 = 1 << 0,
    NETDATA_V4_14 = 1 << 1,
    NETDATA_V4_16 = 1 << 2,
    NETDATA_V4_18 = 1 << 3,
    NETDATA_V5_4  = 1 << 4,
    NETDATA_V5_10 = 1 << 5,
    NETDATA_V5_11 = 1 << 6,
    NETDATA_V5_14 = 1 << 7,
    NETDATA_V5_15 = 1 << 8,
    NETDATA_V5_16 = 1 << 9,
    NETDATA_V6_8  = 1 << 10
};

enum netdata_kernel_idx {
    NETDATA_IDX_V3_10,
    NETDATA_IDX_V4_14,
    NETDATA_IDX_V4_16,
    NETDATA_IDX_V4_18,
    NETDATA_IDX_V5_4 ,
    NETDATA_IDX_V5_10,
    NETDATA_IDX_V5_11,
    NETDATA_IDX_V5_14,
    NETDATA_IDX_V5_15,
    NETDATA_IDX_V5_16,
    NETDATA_IDX_V6_8
};

#define NETDATA_IDX_STR_V3_10 "3.10"
#define NETDATA_IDX_STR_V4_14 "4.14"
#define NETDATA_IDX_STR_V4_16 "4.16"
#define NETDATA_IDX_STR_V4_18 "4.18"
#define NETDATA_IDX_STR_V5_4  "5.4"
#define NETDATA_IDX_STR_V5_10 "5.10"
#define NETDATA_IDX_STR_V5_11 "5.11"
#define NETDATA_IDX_STR_V5_14 "5.14"
#define NETDATA_IDX_STR_V5_15 "5.15"
#define NETDATA_IDX_STR_V5_16 "5.16"
#define NETDATA_IDX_STR_V6_8  "6.8"

/**
 * Minimum value has relationship with libbpf support.
 */
#define NETDATA_MINIMUM_EBPF_KERNEL NETDATA_EBPF_KERNEL_4_11

#define VERSION_STRING_LEN 256
#define EBPF_KERNEL_REJECT_LIST_FILE "ebpf_kernel_reject_list.txt"

#define ND_EBPF_DEFAULT_MIN_PID 1U
#define ND_EBPF_MAP_FD_NOT_INITIALIZED ((int)-1)

typedef struct ebpf_addresses {
    char *function;
    uint32_t hash;
    // We use long as address, because it matches system length
    unsigned long addr;
    uint32_t type;
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
    NETDATA_CONTROLLER_APPS_LEVEL,

    // These index show the number of elements
    // stored inside hash tables.
    //
    // We have indexes to count increase and
    // decrease events, because __sync_fetch_and_sub
    // generates compilation errors.
    NETDATA_CONTROLLER_PID_TABLE_ADD,
    NETDATA_CONTROLLER_PID_TABLE_DEL,
    NETDATA_CONTROLLER_TEMP_TABLE_ADD,
    NETDATA_CONTROLLER_TEMP_TABLE_DEL,

    NETDATA_CONTROLLER_END
};

// Control how Netdata will monitor PIDs (apps and cgroups)
typedef enum netdata_apps_level {
    NETDATA_APPS_LEVEL_REAL_PARENT,
    NETDATA_APPS_LEVEL_PARENT,
    NETDATA_APPS_LEVEL_ALL,

    // Present only in user ring
    NETDATA_APPS_NOT_SET
} netdata_apps_level_t;

typedef struct ebpf_local_maps {
    char *name;
    uint32_t internal_input;
    uint32_t user_input;
    uint32_t type;
    int map_fd;
#ifdef LIBBPF_MAJOR_VERSION
    enum bpf_map_type map_type;
#endif
} ebpf_local_maps_t;

typedef struct ebpf_specify_name {
    char *program_name;
    char *function_to_attach;
    char *optional;
    bool retprobe;
} ebpf_specify_name_t;

typedef enum netdata_ebpf_load_mode {
    EBPF_LOAD_LEGACY = 1<<0,        // Select legacy mode, this means we will load binaries
    EBPF_LOAD_CORE = 1<<1,          // When CO-RE is used, it is necessary to use the source code
    EBPF_LOAD_PLAY_DICE = 1<<2,      // Take a look on environment and choose the best option
    EBPF_LOADED_FROM_STOCK = 1<<3,  // Configuration loaded from Stock file
    EBPF_LOADED_FROM_USER = 1<<4    // Configuration loaded from user
} netdata_ebpf_load_mode_t;
#define NETDATA_EBPF_LOAD_METHODS (EBPF_LOAD_LEGACY|EBPF_LOAD_CORE|EBPF_LOAD_PLAY_DICE)
#define NETDATA_EBPF_LOAD_SOURCE (EBPF_LOADED_FROM_STOCK|EBPF_LOADED_FROM_USER)

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

    uint64_t memlock_kern; // The same information reported by bpftool, but it is not accurated
                           // https://lore.kernel.org/linux-mm/20230112155326.26902-5-laoar.shao@gmail.com/T/
    uint32_t hash_tables; // Number of hash tables used on the system.

    uint32_t hash_percpu; // Number of threads running per cpu maps
    uint32_t hash_unique; // Number of threads running an unique map for all cores.
} ebpf_plugin_stats_t;

typedef enum ebpf_stats_action {
    EBPF_ACTION_STAT_ADD,
    EBPF_ACTION_STAT_REMOVE,
} ebpf_stats_action_t;

typedef enum netdata_apps_integration_flags {
    NETDATA_EBPF_APPS_FLAG_NO,
    NETDATA_EBPF_APPS_FLAG_YES,
    NETDATA_EBPF_APPS_FLAG_CHART_CREATED
} netdata_apps_integration_flags_t;

#define NETDATA_EBPF_CHART_MEM_LENGTH 48
#define NETDATA_EBPF_STAT_DIMENSION_MEMORY "memory"
#define NETDATA_EBPF_STAT_DIMENSION_ARAL "aral"

enum ebpf_threads_status {
    NETDATA_THREAD_EBPF_RUNNING,            // started by plugin
    NETDATA_THREAD_EBPF_FUNCTION_RUNNING,   // started by function
    NETDATA_THREAD_EBPF_STOPPING,           // stopping thread
    NETDATA_THREAD_EBPF_STOPPED,            // thread stopped
    NETDATA_THREAD_EBPF_NOT_RUNNING         // thread was never started
};

enum ebpf_global_table_values {
    NETDATA_EBPF_GLOBAL_TABLE_PID_TABLE_ADD, // Count elements added inside PID table
    NETDATA_EBPF_GLOBAL_TABLE_PID_TABLE_DEL, // Count elements removed from PID table
    NETDATA_EBPF_GLOBAL_TABLE_TEMP_TABLE_ADD, // Count elements added inside TEMP table
    NETDATA_EBPF_GLOBAL_TABLE_TEMP_TABLE_DEL,  // Count elements removed from TEMP table

    NETDATA_EBPF_GLOBAL_TABLE_STATUS_END
};

typedef uint64_t netdata_idx_t;

typedef struct ebpf_module {
    // Constants used with module
    struct {
        const char *thread_name;
        const char *config_name;
        const char *thread_description;
    } info;

    // Helpers used with plugin
    struct {
        void *(*start_routine)(void *);                             // the thread function
        void (*apps_routine)(struct ebpf_module *em, void *ptr);    // the apps charts
        void (*fnct_routine)(BUFFER *bf, struct ebpf_module *em);   // the function used for exteernal requests
        const char *fcnt_name;                                      // name given to cloud
        const char *fcnt_desc;                                      // description given about function
        const char *fcnt_thread_chart_name;
        int order_thread_chart;
        const char *fcnt_thread_lifetime_name;
        int order_thread_lifetime;
    } functions;

    enum ebpf_threads_status enabled;
    int update_every;
    int global_charts;
    netdata_apps_integration_flags_t apps_charts;
    netdata_apps_level_t apps_level;
    int cgroup_charts;
    netdata_run_mode_t mode;
    uint32_t thread_id;
    int optional;
    ebpf_local_maps_t *maps;
    ebpf_specify_name_t *names;
    uint32_t pid_map_size;
    struct config *cfg;
    const char *config_file;
    uint64_t kernels;
    netdata_ebpf_load_mode_t load;
    netdata_ebpf_targets_t *targets;
    struct bpf_link **probe_links;
    struct bpf_object *objects;
    struct netdata_static_thread *thread;

    // charts
    char memory_usage[NETDATA_EBPF_CHART_MEM_LENGTH];
    char memory_allocations[NETDATA_EBPF_CHART_MEM_LENGTH];
    int maps_per_core;

    // period to run
    uint32_t running_time; // internal usage, this is used to reset a value when a new request happens.
    uint32_t lifetime;

    netdata_idx_t hash_table_stats[NETDATA_EBPF_GLOBAL_TABLE_STATUS_END];
} ebpf_module_t;

#define EBPF_DEFAULT_LIFETIME 300
// This will be present until all functions are merged. The deadline is planned for 68 years since plugin start
#define EBPF_NON_FUNCTION_LIFE_TIME UINT_MAX

int ebpf_get_kernel_version();
int get_redhat_release();
char *ebpf_kernel_suffix(int version, int isrh);
struct bpf_link **ebpf_load_program(char *plugins_dir, ebpf_module_t *em, int kver, int is_rhf,
                                           struct bpf_object **obj);

void ebpf_mount_config_name(char *filename, size_t length, char *path, const char *config);
int ebpf_load_config(struct config *config, char *filename);
void ebpf_update_module(ebpf_module_t *em, struct btf *btf_file, int kver, int is_rh);
void ebpf_update_names(ebpf_specify_name_t *opt, ebpf_module_t *em);
void ebpf_adjust_apps_cgroup(ebpf_module_t *em, netdata_ebpf_program_loaded_t mode);
char *ebpf_find_symbol(char *search);
void ebpf_load_addresses(ebpf_addresses_t *fa, int fd);
void ebpf_fill_algorithms(int *algorithms, size_t length, int algorithm);
char **ebpf_fill_histogram_dimension(size_t maximum);
void ebpf_update_stats(ebpf_plugin_stats_t *report, ebpf_module_t *em);
void ebpf_update_controller(int fd, ebpf_module_t *em);
void ebpf_update_map_size(struct bpf_map *map, ebpf_local_maps_t *lmap, ebpf_module_t *em, const char *map_name);

// Histogram
#define NETDATA_EBPF_HIST_MAX_BINS 24UL
#define NETDATA_DISK_MAX 256U
#define NETDATA_DISK_HISTOGRAM_LENGTH (NETDATA_DISK_MAX * NETDATA_EBPF_HIST_MAX_BINS)

typedef struct netdata_ebpf_histogram {
    char *name;
    char *title;
    char *ctx;
    int order;
    uint64_t histogram[NETDATA_EBPF_HIST_MAX_BINS];
} netdata_ebpf_histogram_t;

enum fs_btf_counters {
    NETDATA_KEY_BTF_READ,
    NETDATA_KEY_BTF_WRITE,
    NETDATA_KEY_BTF_OPEN,
    NETDATA_KEY_BTF_SYNC_ATTR,
    NETDATA_KEY_BTF_OPEN2,

    NETDATA_FS_BTF_END
};

typedef struct ebpf_filesystem_partitions {
    char *filesystem;
    char *optional_filesystem;
    char *family;
    char *family_name;
    struct bpf_object *objects;
    struct bpf_link **probe_links;

    netdata_ebpf_histogram_t hread;
    netdata_ebpf_histogram_t hwrite;
    netdata_ebpf_histogram_t hopen;
    netdata_ebpf_histogram_t hadditional;

    uint32_t flags;
    uint32_t enabled;

    ebpf_addresses_t addresses;
    uint64_t kernels;
    ebpf_local_maps_t *fs_maps;

    // BPF structure
#ifdef LIBBPF_MAJOR_VERSION
    struct filesystem_bpf *fs_obj;
#else
    void *fs_obj;
#endif
    const char *functions[NETDATA_FS_BTF_END];
} ebpf_filesystem_partitions_t;

typedef struct ebpf_sync_syscalls {
    char *syscall;
    int enabled;
    uint32_t flags;

    // BTF structure
    struct bpf_object *objects;
    struct bpf_link **probe_links;

    // BPF structure
#ifdef LIBBPF_MAJOR_VERSION
    struct sync_bpf *sync_obj;
#else
    void *sync_obj;
#endif
    ebpf_local_maps_t *sync_maps;
} ebpf_sync_syscalls_t;

void ebpf_histogram_dimension_cleanup(char **ptr, size_t length);

// Tracepoint helpers
// For more information related to tracepoints read https://www.kernel.org/doc/html/latest/trace/tracepoints.html
int ebpf_is_tracepoint_enabled(char *subsys, char *eventname);
int ebpf_enable_tracing_values(char *subsys, char *eventname);
int ebpf_disable_tracing_values(char *subsys, char *eventname);

// BTF Section
#define EBPF_DEFAULT_BTF_FILE "vmlinux"
#define EBPF_DEFAULT_BTF_PATH "/sys/kernel/btf"
#define EBPF_DEFAULT_ERROR_MSG "Cannot open or load BPF file for thread"

// BTF helpers
#define NETDATA_EBPF_MAX_SYSCALL_LENGTH 255

netdata_ebpf_load_mode_t epbf_convert_string_to_load_mode(const char *str);
netdata_ebpf_program_loaded_t ebpf_convert_core_type(const char *str, netdata_run_mode_t lmode);
void ebpf_select_host_prefix(char *output, size_t length, char *syscall, int kver);
#ifdef LIBBPF_MAJOR_VERSION
void ebpf_adjust_thread_load(ebpf_module_t *mod, struct btf *file);
struct btf *ebpf_parse_btf_file(const char *filename);
struct btf *ebpf_load_btf_file(const char *path, const char *filename);
int ebpf_is_function_inside_btf(struct btf *file, char *function);
void ebpf_update_map_type(struct bpf_map *map, ebpf_local_maps_t *w);
void ebpf_define_map_type(ebpf_local_maps_t *maps, int maps_per_core, int kver);
#endif

void ebpf_update_kernel_memory_with_vector(ebpf_plugin_stats_t *report, ebpf_local_maps_t *maps,
                                           ebpf_stats_action_t action);
void ebpf_update_kernel_memory(ebpf_plugin_stats_t *report, ebpf_local_maps_t *map, ebpf_stats_action_t action);
int ebpf_statistic_create_aral_chart(char *name, ebpf_module_t *em);
void ebpf_statistic_obsolete_aral_chart(ebpf_module_t *em, int prio);
void ebpf_send_data_aral_chart(ARAL *memory, ebpf_module_t *em);

int ebpf_can_plugin_load_code(int kver, char *plugin_name);
int ebpf_adjust_memory_limit();

#ifdef LIBBPF_MAJOR_VERSION
static inline int netdata_silent_libbpf_vfprintf(enum libbpf_print_level level __maybe_unused,
                                                 const char *format __maybe_unused,
                                                 va_list args __maybe_unused)
{
     return 0;
}
#endif

#endif /* NETDATA_EBPF_H */
