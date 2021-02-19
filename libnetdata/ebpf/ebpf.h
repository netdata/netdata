// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_H
#define NETDATA_EBPF_H 1

#include <bpf/bpf.h>
#include <bpf/libbpf.h>

#define NETDATA_DEBUGFS "/sys/kernel/debug/tracing/"

/**
 * The next magic number is got doing the following math:
 *  294960 = 4*65536 + 11*256 + 0
 *
 *  For more details, please, read /usr/include/linux/version.h
 */
#define NETDATA_MINIMUM_EBPF_KERNEL 264960

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
 *  Kernel 4.17
 *
 *  266496 = 4*65536 + 17*256
 */
#define NETDATA_EBPF_KERNEL_4_17 266496

/**
 *  Kernel 4.15
 *
 *  265984 = 4*65536 + 15*256
 */
#define NETDATA_EBPF_KERNEL_4_15 265984

/**
 *  Kernel 4.11
 *
 *  264960 = 4*65536 + 15*256
 */
#define NETDATA_EBPF_KERNEL_4_11 264960

#define VERSION_STRING_LEN 256
#define EBPF_KERNEL_REJECT_LIST_FILE "ebpf_kernel_reject_list.txt"

typedef struct netdata_ebpf_events {
    char type;
    char *name;
} netdata_ebpf_events_t;

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

typedef struct ebpf_module {
    const char *thread_name;
    const char *config_name;
    int enabled;
    void *(*start_routine)(void *);
    int update_time;
    int global_charts;
    int apps_charts;
    netdata_run_mode_t mode;
    netdata_ebpf_events_t *probes;
    uint32_t thread_id;
    int optional;
} ebpf_module_t;

#define NETDATA_MAX_PROBES 64

extern int clean_kprobe_events(FILE *out, int pid, netdata_ebpf_events_t *ptr);
extern int get_kernel_version(char *out, int size);
extern int get_redhat_release();
extern int has_condition_to_run(int version);
extern char *ebpf_kernel_suffix(int version, int isrh);
extern int ebpf_update_kernel(ebpf_data_t *ef);
extern struct bpf_link **ebpf_load_program(char *plugins_dir,
                             ebpf_module_t *em,
                             char *kernel_string,
                             struct bpf_object **obj,
                             int *map_fd);

#endif /* NETDATA_EBPF_H */
