#ifndef _NETDATA_EBPF_H_
# define _NETDATA_EBPF_H_ 1

# define NETDATA_DEBUGFS "/sys/kernel/debug/tracing/"

typedef struct netdata_ebpf_events {
    char type;
    char *name;

} netdata_ebpf_events_t;

int clean_kprobe_events(FILE *out, int pid, netdata_ebpf_events_t *ptr);
int has_condition_to_run();

#endif
