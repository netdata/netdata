#ifndef _NETDATA_EBPF_H_
# define _NETDATA_EBPF_H_ 1

# define NETDATA_DEBUGFS "/sys/kernel/debug/tracing/"

/**
 * The next magic number is got doing the following math:
 *  294960 = 4*65536 + 11*256 + 0
 *
 *  For more details, please, read /usr/include/linux/version.h
 */
# define NETDATA_MINIMUM_EBPF_KERNEL 264960

/**
 * The RedHat magic number was got doing:
 *
 * 1797 = 7*256 + 5
 *
 *  For more details, please, read /usr/include/linux/version.h
 *  in any Red Hat installation.
 */
# define NETDATA_MINIMUM_RH_VERSION 1797

/**
 * 2048 = 8*256 + 0
 */
# define NETDATA_RH_8 2048

/**
 *  Kernel 4.17
 *
 *  266496 = 4*65536 + 17*256
 */
# define NETDATA_EBPF_KERNEL_4_17 266496

/**
 *  Kernel 4.15
 *
 *  265984 = 4*65536 + 15*256
 */
# define NETDATA_EBPF_KERNEL_4_15 265984

/**
 *  Kernel 4.11
 *
 *  264960 = 4*65536 + 15*256
 */
# define NETDATA_EBPF_KERNEL_4_11 264960

typedef struct netdata_ebpf_events {
    char type;
    char *name;
} netdata_ebpf_events_t;

typedef struct ebpf_functions {
    void *libnetdata;
    int (*load_bpf_file)(int *, char *, int);
    //Libbpf (It is necessary to have at least kernel 4.10)
    int (*bpf_map_lookup_elem)(int, const void *, void *);
    int (*bpf_map_delete_elem)(int fd, const void *key);
    int (*bpf_map_get_next_key)(int fd, const void *key, void *next_key);

    int *map_fd;

    char *kernel_string;
    uint32_t running_on_kernel;
    int isrh;
} ebpf_functions_t;

extern int clean_kprobe_events(FILE *out, int pid, netdata_ebpf_events_t *ptr);
extern int get_kernel_version(char *out, int size);
extern int get_redhat_release();
extern int has_condition_to_run(int version);
extern char *ebpf_library_suffix(int version, int isrh);
extern int ebpf_load_libraries(ebpf_functions_t *ef, char *libbase, char *pluginsdir);
extern int ebpf_load_program(char *plugins_dir,
                             int event_id, int mode,
                             char *kernel_string,
                             const char *name,
                             int *map_fd,
                             int (*load_bpf_file)(int *,char *, int));

#endif
