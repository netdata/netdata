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


typedef struct netdata_ebpf_events {
    char type;
    char *name;

} netdata_ebpf_events_t;

extern int clean_kprobe_events(FILE *out, int pid, netdata_ebpf_events_t *ptr);
extern int get_kernel_version();
extern int get_redhat_release();
extern int has_condition_to_run(int version);

#endif
